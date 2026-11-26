package http_test

import (
	"context"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// fakeSettlementRepository keeps the settlements of each practice in memory. It reads the
// mutuales from the insurer double and the sessions from the billing double, the way the adapter
// reads them from the tables, and it refuses a session that an active settlement already holds.
type fakeSettlementRepository struct {
	insurers    *fakeInsurerRepository
	patients    *fakePatientRepository
	sessions    *fakeBillingRepository
	settlements map[string]map[billing.SettlementID]billing.Settlement
}

var _ port.SettlementRepository = (*fakeSettlementRepository)(nil)

func newFakeSettlementRepository(insurers *fakeInsurerRepository, patients *fakePatientRepository, sessions *fakeBillingRepository) *fakeSettlementRepository {
	return &fakeSettlementRepository{
		insurers:    insurers,
		patients:    patients,
		sessions:    sessions,
		settlements: map[string]map[billing.SettlementID]billing.Settlement{},
	}
}

func (r *fakeSettlementRepository) Insurer(ctx context.Context, insurerID string) (billing.SettlementInsurer, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.SettlementInsurer{}, billing.ErrTenantScopeMissing
	}
	subject, found := r.insurers.stored[insurer.ID(insurerID)]
	if !found || string(subject.TenantID) != tenantID {
		return billing.SettlementInsurer{}, billing.ErrInsurerNotVisible
	}
	return toSettlementInsurer(subject), nil
}

func (r *fakeSettlementRepository) Insurers(ctx context.Context) ([]billing.SettlementInsurer, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	out := []billing.SettlementInsurer{}
	for _, subject := range r.insurers.stored {
		if string(subject.TenantID) == tenantID && subject.IsActive {
			out = append(out, toSettlementInsurer(subject))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *fakeSettlementRepository) SettlableSessions(ctx context.Context, insurerID string, from, to time.Time) ([]billing.SettlableSession, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	liquidated := r.activeAppointments(tenantID)
	out := []billing.SettlableSession{}
	for _, session := range r.sessions.sessions[tenantID] {
		if session.Status != billing.AppointmentAttended || liquidated[session.ID] {
			continue
		}
		if session.StartsAt.Before(from) || !session.StartsAt.Before(to) {
			continue
		}
		subject, err := r.patients.ByID(ctx, patient.ID(session.PatientID))
		payer, payerErr := r.sessions.clients.ByID(ctx, client.ID(subject.ClientID))
		if err != nil || payerErr != nil || subject.CoverageKind != patient.CoverageInsurer || subject.InsurerID == nil || *subject.InsurerID != insurerID {
			continue
		}
		out = append(out, billing.SettlableSession{
			AppointmentID:    session.ID,
			PatientID:        session.PatientID,
			PatientName:      subject.FullName,
			MemberNumber:     settlementText(subject.MemberNumber),
			ProfessionalID:   session.ProfessionalID,
			ProfessionalName: session.ProfessionalName,
			Modality:         session.Modality,
			StartsAt:         session.StartsAt,
			Currency:         string(payer.BillingCurrency),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out, nil
}

func (r *fakeSettlementRepository) Issue(ctx context.Context, settlement billing.Settlement) (billing.Settlement, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Settlement{}, billing.ErrTenantScopeMissing
	}
	liquidated := r.activeAppointments(tenantID)
	for _, item := range settlement.Items {
		if liquidated[item.AppointmentID] {
			return billing.Settlement{}, billing.ErrAppointmentAlreadySettled
		}
	}
	if r.settlements[tenantID] == nil {
		r.settlements[tenantID] = map[billing.SettlementID]billing.Settlement{}
	}
	r.settlements[tenantID][settlement.ID] = settlement
	return settlement, nil
}

func (r *fakeSettlementRepository) Void(ctx context.Context, settlement billing.Settlement) (billing.Settlement, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Settlement{}, billing.ErrTenantScopeMissing
	}
	if _, found := r.settlements[tenantID][settlement.ID]; !found {
		return billing.Settlement{}, billing.ErrNotFound
	}
	r.settlements[tenantID][settlement.ID] = settlement
	return settlement, nil
}

func (r *fakeSettlementRepository) ByID(ctx context.Context, id billing.SettlementID) (billing.Settlement, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Settlement{}, billing.ErrTenantScopeMissing
	}
	settlement, found := r.settlements[tenantID][id]
	if !found {
		return billing.Settlement{}, billing.ErrNotFound
	}
	return settlement, nil
}

func (r *fakeSettlementRepository) List(ctx context.Context, filter port.SettlementFilter) ([]billing.Settlement, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	rows := []billing.Settlement{}
	for _, settlement := range r.settlements[tenantID] {
		if filter.InsurerID != "" && settlement.InsurerID != filter.InsurerID {
			continue
		}
		rows = append(rows, settlement)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	if int(filter.Offset) >= len(rows) {
		return []billing.Settlement{}, nil
	}
	rows = rows[filter.Offset:]
	if filter.Limit > 0 && int(filter.Limit) < len(rows) {
		rows = rows[:filter.Limit]
	}
	return rows, nil
}

// activeAppointments returns the sessions that an issued settlement of the practice holds.
func (r *fakeSettlementRepository) activeAppointments(tenantID string) map[string]bool {
	held := map[string]bool{}
	for _, settlement := range r.settlements[tenantID] {
		if settlement.Status != billing.StatusIssued {
			continue
		}
		for _, item := range settlement.Items {
			if item.ReleasedAt == nil {
				held[item.AppointmentID] = true
			}
		}
	}
	return held
}

func toSettlementInsurer(subject insurer.Insurer) billing.SettlementInsurer {
	return billing.SettlementInsurer{ID: string(subject.ID), Name: subject.Name, TaxID: subject.TaxID, Active: subject.IsActive}
}

func settlementText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

const settlementTaxID = "30711223334"

// seedMutualInsurer makes an active mutual of the practice. A nil CUIT is a mutual that cannot be
// settled yet.
func seedMutualInsurer(t *testing.T, h *harness, tenantID, name string, taxID *string) string {
	t.Helper()
	id := uuid.NewString()
	h.insurers.stored[insurer.ID(id)] = insurer.Insurer{
		ID: insurer.ID(id), TenantID: tenant.ID(tenantID), Name: name, TaxID: taxID, IsActive: true,
	}
	return id
}

// seedCoveredPatient makes a patient covered by the mutual, billed in pesos by its client, and returns its id.
func seedCoveredPatient(t *testing.T, h *harness, owner, tenantID, insurerID, fullName, document string) string {
	t.Helper()
	patientID := createPatient(t, h, owner, fullName, document)
	subject := h.repo.rows[tenantID][patient.ID(patientID)]
	member := "AF-" + document
	subject.CoverageKind = patient.CoverageInsurer
	subject.InsurerID = &insurerID
	subject.MemberNumber = &member
	h.repo.rows[tenantID][patient.ID(patientID)] = subject
	return patientID
}

// seedMutualPrice prices the professional's presencial sessions with the mutual, from January.
func seedMutualPrice(t *testing.T, h *harness, tenantID, professionalID, insurerID string, insurerMinor int64) {
	t.Helper()
	id := pricing.ID(uuid.NewString())
	h.tariffs.stored[id] = pricing.Tariff{
		ID:                 id,
		TenantID:           tenant.ID(tenantID),
		CreatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ProfessionalUserID: professionalID,
		Coverage:           patient.CoverageInsurer,
		InsurerID:          &insurerID,
		Modality:           schedule.ModalityPresencial,
		Currency:           patient.CurrencyARS,
		ValidFrom:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		InsurerMinor:       insurerMinor,
		PatientMinor:       150000,
	}
}

// seedAttendedSession puts an attended presencial session of the patient in the billing double.
func seedAttendedSession(t *testing.T, h *harness, tenantID, patientID string, professional billing.Professional, startsAt time.Time) string {
	t.Helper()
	return seedSession(t, h, tenantID, billing.Appointment{
		PatientID:        patientID,
		ProfessionalID:   professional.ID,
		ProfessionalName: professional.FullName,
		Modality:         billing.ModalityPresencial,
		Status:           billing.AppointmentAttended,
		StartsAt:         startsAt,
	})
}

func someTaxID(value string) *string { return &value }

func issueSettlement(t *testing.T, h *harness, token, insurerID, period string) (int, map[string]any) {
	t.Helper()
	return h.do(t, http.MethodPost, "/api/v1/billing/settlements", token, map[string]any{"insurerId": insurerID, "period": period})
}

func previewSettlement(t *testing.T, h *harness, token, insurerID, period string) (int, map[string]any) {
	t.Helper()
	return h.do(t, http.MethodGet, "/api/v1/billing/settlements/preview?insurerId="+insurerID+"&period="+period, token, nil)
}

func listOf(body map[string]any, key string) []any {
	items, _ := body[key].([]any)
	return items
}

// TestTheOwnerSettlesTheAttendedSessionsOfAMutualForAMonth: the attended sessions of the mutual's
// patients in the month are valued at the mutual's share and written as one settlement. The
// sessions of other months, of private patients and of other mutuales are left out.
func TestTheOwnerSettlesTheAttendedSessionsOfAMutualForAMonth(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))
	professional := seedProfessional(t, h, tenantA, "Dra. Mutual")
	seedMutualPrice(t, h, tenantA, professional.ID, mutual, 800000)
	ana := seedCoveredPatient(t, h, owner, tenantA, mutual, "Ana Mutual", "30111222")
	luis := seedCoveredPatient(t, h, owner, tenantA, mutual, "Luis Mutual", "30333444")
	private := createPatient(t, h, owner, "Sofía Particular", "30555666")
	seedAttendedSession(t, h, tenantA, ana, professional, time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC))
	seedAttendedSession(t, h, tenantA, luis, professional, time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC))
	seedAttendedSession(t, h, tenantA, ana, professional, time.Date(2026, 9, 28, 15, 0, 0, 0, time.UTC))
	seedAttendedSession(t, h, tenantA, private, professional, time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC))

	status, body := issueSettlement(t, h, owner, mutual, "2026-10")
	if status != http.StatusCreated {
		t.Fatalf("issue = %d, want 201: %v", status, body)
	}
	if body["status"] != "issued" || body["period"] != "2026-10" || body["insurerName"] != "OSDE" || body["insurerTaxId"] != settlementTaxID {
		t.Errorf("settlement header = %v", body)
	}
	if items := listOf(body, "items"); len(items) != 2 {
		t.Fatalf("items = %d, want 2 (the October sessions of the mutual's patients): %v", len(items), body)
	}
	for _, row := range listOf(body, "items") {
		if copago := row.(map[string]any)["copagoMinor"]; copago != float64(150000) {
			t.Errorf("item copagoMinor = %v, want the tariff's copago 150000", copago)
		}
	}
	totals := listOf(body, "totals")
	if len(totals) != 1 {
		t.Fatalf("totals = %v, want one currency", totals)
	}
	total, _ := totals[0].(map[string]any)
	if total["currency"] != "ARS" || total["sessions"] != float64(2) || total["amountMinor"] != float64(1600000) {
		t.Errorf("total = %v, want 2 ARS sessions for 1600000", total)
	}

	status, list := h.do(t, http.MethodGet, "/api/v1/billing/settlements?insurerId="+mutual, owner, nil)
	if status != http.StatusOK || len(listOf(list, "items")) != 1 {
		t.Errorf("list = %d %v, want the one settlement", status, list)
	}
}

// TestASessionIsNeverLiquidatedTwice: once a settlement holds a session, the month offers nothing
// more for it, so a second settlement of the same month has nothing to write.
func TestASessionIsNeverLiquidatedTwice(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))
	professional := seedProfessional(t, h, tenantA, "Dra. Mutual")
	seedMutualPrice(t, h, tenantA, professional.ID, mutual, 800000)
	ana := seedCoveredPatient(t, h, owner, tenantA, mutual, "Ana Mutual", "30111222")
	seedAttendedSession(t, h, tenantA, ana, professional, time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC))

	if status, body := issueSettlement(t, h, owner, mutual, "2026-10"); status != http.StatusCreated {
		t.Fatalf("first issue = %d, want 201: %v", status, body)
	}
	if status, body := issueSettlement(t, h, owner, mutual, "2026-10"); status != http.StatusUnprocessableEntity {
		t.Errorf("second issue = %d, want 422 (nothing left to settle): %v", status, body)
	}
	status, preview := previewSettlement(t, h, owner, mutual, "2026-10")
	if status != http.StatusOK || len(listOf(preview, "sessions")) != 0 {
		t.Errorf("preview after the settlement = %d %v, want no sessions left", status, preview)
	}
}

// TestVoidingASettlementReleasesItsSessionsToBeSettledAgain: a void is final, releases the
// sessions, and the sessions can then be settled by a new settlement.
func TestVoidingASettlementReleasesItsSessionsToBeSettledAgain(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))
	professional := seedProfessional(t, h, tenantA, "Dra. Mutual")
	seedMutualPrice(t, h, tenantA, professional.ID, mutual, 800000)
	ana := seedCoveredPatient(t, h, owner, tenantA, mutual, "Ana Mutual", "30111222")
	luis := seedCoveredPatient(t, h, owner, tenantA, mutual, "Luis Mutual", "30333444")
	seedAttendedSession(t, h, tenantA, ana, professional, time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC))
	seedAttendedSession(t, h, tenantA, luis, professional, time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC))

	status, issued := issueSettlement(t, h, owner, mutual, "2026-10")
	if status != http.StatusCreated {
		t.Fatalf("issue = %d, want 201: %v", status, issued)
	}
	voidPath := "/api/v1/billing/settlements/" + issued["id"].(string) + "/void"

	status, voided := h.do(t, http.MethodPost, voidPath, accountant, map[string]any{"reason": "Lote mal armado"})
	if status != http.StatusOK || voided["status"] != "voided" {
		t.Fatalf("void = %d %v, want 200 voided", status, voided)
	}
	for _, item := range listOf(voided, "items") {
		if item.(map[string]any)["releasedAt"] == nil {
			t.Errorf("item %v was not released by the void", item)
		}
	}
	if status, body := h.do(t, http.MethodPost, voidPath, accountant, map[string]any{"reason": "de nuevo"}); status != http.StatusConflict {
		t.Errorf("second void = %d, want 409: %v", status, body)
	}

	if status, preview := previewSettlement(t, h, owner, mutual, "2026-10"); status != http.StatusOK || len(listOf(preview, "sessions")) != 2 {
		t.Errorf("preview after the void = %d %v, want the two sessions again", status, preview)
	}
	if status, body := issueSettlement(t, h, owner, mutual, "2026-10"); status != http.StatusCreated {
		t.Errorf("re-issue after the void = %d, want 201: %v", status, body)
	}
}

// TestAUnpricedSessionIsListedAndLeftOut: a session the tariff does not price is not settled; it
// is listed as unvalued so the practice sees what the mutual will not pay yet.
func TestAUnpricedSessionIsListedAndLeftOut(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))
	priced := seedProfessional(t, h, tenantA, "Dra. Con tarifa")
	unpriced := seedProfessional(t, h, tenantA, "Dr. Sin tarifa")
	seedMutualPrice(t, h, tenantA, priced.ID, mutual, 800000)
	ana := seedCoveredPatient(t, h, owner, tenantA, mutual, "Ana Mutual", "30111222")
	seedAttendedSession(t, h, tenantA, ana, priced, time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC))
	seedAttendedSession(t, h, tenantA, ana, unpriced, time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC))

	status, preview := previewSettlement(t, h, owner, mutual, "2026-10")
	if status != http.StatusOK || len(listOf(preview, "sessions")) != 1 || len(listOf(preview, "unvalued")) != 1 {
		t.Fatalf("preview = %d %v, want one valued and one unvalued session", status, preview)
	}
	if status, body := issueSettlement(t, h, owner, mutual, "2026-10"); status != http.StatusCreated || len(listOf(body, "items")) != 1 {
		t.Errorf("issue = %d %v, want 201 with the priced session only", status, body)
	}
}

// TestAMutualWithoutItsCUITIsNotSettled: a settlement is addressed to the mutual's CUIT, so a
// mutual without one is listed as not settlable and refused.
func TestAMutualWithoutItsCUITIsNotSettled(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	noCUIT := seedMutualInsurer(t, h, tenantA, "Mutual Sin CUIT", nil)
	professional := seedProfessional(t, h, tenantA, "Dra. Mutual")
	seedMutualPrice(t, h, tenantA, professional.ID, noCUIT, 800000)
	ana := seedCoveredPatient(t, h, owner, tenantA, noCUIT, "Ana Mutual", "30111222")
	seedAttendedSession(t, h, tenantA, ana, professional, time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC))

	status, insurers := h.do(t, http.MethodGet, "/api/v1/billing/settlement-insurers", owner, nil)
	if status != http.StatusOK {
		t.Fatalf("list insurers = %d, want 200: %v", status, insurers)
	}
	items := listOf(insurers, "items")
	if len(items) != 1 || items[0].(map[string]any)["settlable"] != false {
		t.Errorf("insurers = %v, want the mutual not settlable", items)
	}
	if status, body := issueSettlement(t, h, owner, noCUIT, "2026-10"); status != http.StatusUnprocessableEntity {
		t.Errorf("issue = %d, want 422: %v", status, body)
	}
}

// TestOnlyTheBillingRolesSettle: reading and issuing follow billing; a professional does neither.
func TestOnlyTheBillingRolesSettle(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	professional := h.token(t, tenantA, iam.RoleProfessional)
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))

	if status, _ := previewSettlement(t, h, professional, mutual, "2026-10"); status != http.StatusForbidden {
		t.Errorf("professional preview = %d, want 403", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/billing/settlements", professional, nil); status != http.StatusForbidden {
		t.Errorf("professional list = %d, want 403", status)
	}
	if status, _ := issueSettlement(t, h, professional, mutual, "2026-10"); status != http.StatusForbidden {
		t.Errorf("professional issue = %d, want 403", status)
	}
	if status, _ := previewSettlement(t, h, accountant, mutual, "2026-10"); status != http.StatusOK {
		t.Errorf("accountant preview = %d, want 200", status)
	}
}

// TestAPeriodIsAMonth: the period is a calendar month, written YYYY-MM.
func TestAPeriodIsAMonth(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))

	if status, _ := previewSettlement(t, h, owner, mutual, "2026-13"); status != http.StatusUnprocessableEntity {
		t.Errorf("preview of month 13 = %d, want 422", status)
	}
	if status, _ := issueSettlement(t, h, owner, mutual, "2026-10-01"); status != http.StatusUnprocessableEntity {
		t.Errorf("issue of a day = %d, want 422", status)
	}
}

// TestAMissingSettlementIsNotFoundAndVoidNeedsAReason: an unknown settlement answers 404, and a
// void without a reason is refused.
func TestAMissingSettlementIsNotFoundAndVoidNeedsAReason(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))
	professional := seedProfessional(t, h, tenantA, "Dra. Mutual")
	seedMutualPrice(t, h, tenantA, professional.ID, mutual, 800000)
	ana := seedCoveredPatient(t, h, owner, tenantA, mutual, "Ana Mutual", "30111222")
	seedAttendedSession(t, h, tenantA, ana, professional, time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC))

	missing := uuid.NewString()
	if status, _ := h.do(t, http.MethodGet, "/api/v1/billing/settlements/"+missing, owner, nil); status != http.StatusNotFound {
		t.Errorf("get missing = %d, want 404", status)
	}
	if status, _ := h.do(t, http.MethodPost, "/api/v1/billing/settlements/"+missing+"/void", owner, map[string]any{"reason": "Error"}); status != http.StatusNotFound {
		t.Errorf("void missing = %d, want 404", status)
	}

	status, issued := issueSettlement(t, h, owner, mutual, "2026-10")
	if status != http.StatusCreated {
		t.Fatalf("issue = %d, want 201: %v", status, issued)
	}
	voidPath := "/api/v1/billing/settlements/" + issued["id"].(string) + "/void"
	for _, reason := range []string{"", "   "} {
		if status, body := h.do(t, http.MethodPost, voidPath, owner, map[string]any{"reason": reason}); status != http.StatusUnprocessableEntity {
			t.Errorf("void with reason %q = %d, want 422: %v", reason, status, body)
		}
	}
}

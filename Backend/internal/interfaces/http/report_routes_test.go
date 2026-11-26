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
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/report"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// fakeReportRepository reads the vouchers and the sessions of the billing double and the
// patients of the patient double, and filters them the way the adapter's queries do.
type fakeReportRepository struct {
	patients *fakePatientRepository
	billing  *fakeBillingRepository
	insurers *fakeInsurerRepository
}

var _ port.ReportRepository = (*fakeReportRepository)(nil)

func newFakeReportRepository(patients *fakePatientRepository, billing *fakeBillingRepository, insurers *fakeInsurerRepository) *fakeReportRepository {
	return &fakeReportRepository{patients: patients, billing: billing, insurers: insurers}
}

func matchesFilter(filter report.Filter, line report.Line) bool {
	return (filter.ProfessionalID == "" || line.ProfessionalID == filter.ProfessionalID) &&
		(filter.Coverage == "" || line.Coverage == filter.Coverage) &&
		(filter.InsurerID == "" || line.InsurerID == filter.InsurerID) &&
		(filter.Modality == "" || line.Modality == filter.Modality)
}

func (r *fakeReportRepository) BilledLines(ctx context.Context, window report.Window, filter report.Filter) ([]report.BilledLine, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	out := []report.BilledLine{}
	for _, voucher := range r.billing.vouchers[tenantID] {
		if voucher.Status != billing.StatusIssued || voucher.IssuedAt.Before(window.From) || !voucher.IssuedAt.Before(window.To) {
			continue
		}
		line := report.Line{
			At:               voucher.IssuedAt,
			Currency:         voucher.Currency,
			Coverage:         string(voucher.Photo.Coverage),
			InsurerID:        voucher.Photo.InsurerID,
			InsurerName:      voucher.Photo.InsurerName,
			Modality:         voucher.Photo.Modality,
			ProfessionalID:   voucher.Photo.ProfessionalID,
			ProfessionalName: voucher.Photo.ProfessionalName,
		}
		if matchesFilter(filter, line) {
			out = append(out, report.BilledLine{Line: line, AmountMinor: voucher.AmountMinor})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

func (r *fakeReportRepository) PendingSessions(ctx context.Context, window report.Window, filter report.Filter) ([]report.Line, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	out := []report.Line{}
	for _, session := range r.billing.sessions[tenantID] {
		if session.Status != billing.AppointmentAttended || r.billing.billedBy(tenantID, session.ID) != "" {
			continue
		}
		if session.StartsAt.Before(window.From) || !session.StartsAt.Before(window.To) {
			continue
		}
		subject, err := r.patients.ByID(ctx, patient.ID(session.PatientID))
		if err != nil {
			continue
		}
		line := report.Line{
			At:               session.StartsAt,
			Coverage:         string(subject.CoverageKind),
			Modality:         session.Modality,
			ProfessionalID:   session.ProfessionalID,
			ProfessionalName: session.ProfessionalName,
		}
		// Since T81 the currency is reported from the billing client's row, like the
		// real adapter does.
		if payer, err := r.billing.clients.ByID(ctx, client.ID(subject.ClientID)); err == nil {
			line.Currency = string(payer.BillingCurrency)
		}
		if subject.InsurerID != nil {
			line.InsurerID = *subject.InsurerID
			for id, stored := range r.insurers.stored {
				if string(id) == *subject.InsurerID {
					line.InsurerName = stored.Name
				}
			}
		}
		if matchesFilter(filter, line) {
			out = append(out, line)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
}

// seedIssuedVoucher writes an issued voucher straight into the billing double, with the photo
// and the instant the test needs.
func seedIssuedVoucher(h *harness, tenantID string, issuedAt time.Time, currency string, amount int64, photo billing.Photo) billing.VoucherID {
	id := billing.VoucherID(uuid.NewString())
	if h.vouchers.vouchers[tenantID] == nil {
		h.vouchers.vouchers[tenantID] = map[billing.VoucherID]billing.Voucher{}
	}
	h.vouchers.vouchers[tenantID][id] = billing.Voucher{
		ID: id, Status: billing.StatusIssued, IssuedAt: issuedAt, Currency: currency, AmountMinor: amount, Photo: photo,
	}
	return id
}

func voidSeededVoucher(h *harness, tenantID string, id billing.VoucherID) {
	voucher := h.vouchers.vouchers[tenantID][id]
	voucher.Status = billing.StatusVoided
	h.vouchers.vouchers[tenantID][id] = voucher
}

func professionalPhoto(professional billing.Professional) billing.Photo {
	return billing.Photo{
		Coverage: billing.CoveragePrivate, Modality: billing.ModalityPresencial,
		ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
	}
}

func billingReport(t *testing.T, h *harness, token, query string) (int, map[string]any) {
	t.Helper()
	return h.do(t, http.MethodGet, "/api/v1/reports/billing?"+query, token, nil)
}

// amountOf reads the amount of one currency out of a list of money entries.
func amountOf(t *testing.T, body any, currency string) (amount int64, count int) {
	t.Helper()
	entries, _ := body.([]any)
	for _, raw := range entries {
		entry, _ := raw.(map[string]any)
		if entry["currency"] == currency {
			value, _ := entry["amountMinor"].(float64)
			number, _ := entry["count"].(float64)
			return int64(value), int(number)
		}
	}
	return 0, 0
}

func totalsOf(body map[string]any) map[string]any {
	totals, _ := body["totals"].(map[string]any)
	return totals
}

const novemberQuery = "from=2026-11-01&to=2026-11-30"

// TestTheReportAddsWhatWasBilledAndWhatWasAttendedWithoutBilling: the issued vouchers of the range
// are billed, a voided one is not, one outside the range is not, and the attended sessions
// without a voucher are valued at the tariff in force, with the unpriced ones counted apart.
func TestTheReportAddsWhatWasBilledAndWhatWasAttendedWithoutBilling(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	professional := seedProfessional(t, h, tenantA, "Dra. Demo")
	seedParticularTariff(t, h, tenantA, professional.ID, 1500000)

	photo := professionalPhoto(professional)
	seedIssuedVoucher(h, tenantA, time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC), "ARS", 1000000, photo)
	seedIssuedVoucher(h, tenantA, time.Date(2026, 11, 6, 15, 0, 0, 0, time.UTC), "ARS", 500000, photo)
	seedIssuedVoucher(h, tenantA, time.Date(2026, 11, 7, 15, 0, 0, 0, time.UTC), "USD", 8000, billing.Photo{})
	voided := seedIssuedVoucher(h, tenantA, time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC), "ARS", 999999, photo)
	voidSeededVoucher(h, tenantA, voided)
	seedIssuedVoucher(h, tenantA, time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC), "ARS", 777777, photo)

	ana := createPatient(t, h, owner, "Ana Paciente", "30111222")
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: ana, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
		StartsAt: time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC),
	})
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: ana, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
		StartsAt: time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC),
	})
	// Online has no tariff: counted apart. A scheduled session is not attended: not counted.
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: ana, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityOnline, Status: billing.AppointmentAttended,
		StartsAt: time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC),
	})
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: ana, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: "scheduled",
		StartsAt: time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC),
	})

	status, body := billingReport(t, h, owner, novemberQuery)
	if status != http.StatusOK {
		t.Fatalf("report = %d, want 200: %v", status, body)
	}
	if body["scope"] != "practice" || body["groupBy"] != "none" || body["timezone"] != "America/Argentina/Buenos_Aires" {
		t.Errorf("frame = %v", body)
	}
	totals := totalsOf(body)
	if amount, count := amountOf(t, totals["billed"], "ARS"); amount != 1500000 || count != 2 {
		t.Errorf("billed ARS = %d in %d vouchers, want 1500000 in 2 (no voided, none outside the range)", amount, count)
	}
	if amount, count := amountOf(t, totals["billed"], "USD"); amount != 8000 || count != 1 {
		t.Errorf("billed USD = %d in %d, want 8000 in 1: currencies are never added", amount, count)
	}
	if amount, count := amountOf(t, totals["pending"], "ARS"); amount != 3000000 || count != 2 {
		t.Errorf("pending ARS = %d in %d sessions, want 3000000 in 2", amount, count)
	}
	if unvalued, _ := totals["unvaluedSessions"].(float64); unvalued != 1 {
		t.Errorf("unvalued = %v, want 1", totals["unvaluedSessions"])
	}
	if rows := listOf(body, "rows"); len(rows) != 0 {
		t.Errorf("no cut must give no rows, got %v", rows)
	}
}

// TestASessionBilledByAnIssuedVoucherIsNotPending: billing a session moves it from pending to
// billed; voiding the voucher moves it back.
func TestASessionBilledByAnIssuedVoucherIsNotPending(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	professional := seedProfessional(t, h, tenantA, "Dra. Demo")
	seedParticularTariff(t, h, tenantA, professional.ID, 1500000)
	ana := createPatient(t, h, owner, "Ana Paciente", "30111222")
	sessionID := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: ana, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
		StartsAt: time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC),
	})

	_, body := billingReport(t, h, owner, novemberQuery)
	if amount, _ := amountOf(t, totalsOf(body)["pending"], "ARS"); amount != 1500000 {
		t.Fatalf("pending before billing = %d, want 1500000", amount)
	}

	id := seedIssuedVoucher(h, tenantA, time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC), "ARS", 1500000, professionalPhoto(professional))
	voucher := h.vouchers.vouchers[tenantA][id]
	voucher.AppointmentID = sessionID
	h.vouchers.vouchers[tenantA][id] = voucher

	_, body = billingReport(t, h, owner, novemberQuery)
	if amount, _ := amountOf(t, totalsOf(body)["pending"], "ARS"); amount != 0 {
		t.Errorf("pending after billing = %d, want 0", amount)
	}
	if amount, _ := amountOf(t, totalsOf(body)["billed"], "ARS"); amount != 1500000 {
		t.Errorf("billed after billing = %d, want 1500000", amount)
	}

	voidSeededVoucher(h, tenantA, id)
	_, body = billingReport(t, h, owner, novemberQuery)
	if amount, _ := amountOf(t, totalsOf(body)["pending"], "ARS"); amount != 1500000 {
		t.Errorf("pending after the void = %d, want 1500000: a voided voucher bills nothing", amount)
	}
	if amount, _ := amountOf(t, totalsOf(body)["billed"], "ARS"); amount != 0 {
		t.Errorf("billed after the void = %d, want 0", amount)
	}
}

// TestTheRangeIsReadInThePracticeTimeZone: a voucher at 23:30 in Buenos Aires on 31 October is
// already 1 November in UTC, and still belongs to October.
func TestTheRangeIsReadInThePracticeTimeZone(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	seedIssuedVoucher(h, tenantA, time.Date(2026, 11, 1, 2, 30, 0, 0, time.UTC), "ARS", 100, billing.Photo{})

	_, october := billingReport(t, h, owner, "from=2026-10-01&to=2026-10-31")
	if amount, _ := amountOf(t, totalsOf(october)["billed"], "ARS"); amount != 100 {
		t.Errorf("October = %d, want 100", amount)
	}
	_, november := billingReport(t, h, owner, novemberQuery)
	if amount, _ := amountOf(t, totalsOf(november)["billed"], "ARS"); amount != 0 {
		t.Errorf("November = %d, want 0", amount)
	}
}

// TestTheRowsOfACutAddUpToTheTotals: cutting by professional, every voucher lands in a row, the
// ones that name no professional under the empty key, and the rows add up to the totals.
func TestTheRowsOfACutAddUpToTheTotals(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	ana := seedProfessional(t, h, tenantA, "Dra. Ana")
	bruno := seedProfessional(t, h, tenantA, "Dr. Bruno")
	at := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	seedIssuedVoucher(h, tenantA, at, "ARS", 100, professionalPhoto(ana))
	seedIssuedVoucher(h, tenantA, at, "ARS", 200, professionalPhoto(ana))
	seedIssuedVoucher(h, tenantA, at, "ARS", 400, professionalPhoto(bruno))
	seedIssuedVoucher(h, tenantA, at, "ARS", 800, billing.Photo{})

	status, body := billingReport(t, h, owner, novemberQuery+"&groupBy=professional")
	if status != http.StatusOK {
		t.Fatalf("report = %d, want 200: %v", status, body)
	}
	rows := listOf(body, "rows")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (two professionals and the loose charge): %v", len(rows), rows)
	}
	var sum int64
	labels := []string{}
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		amount, _ := amountOf(t, row["billed"], "ARS")
		sum += amount
		label, _ := row["label"].(string)
		labels = append(labels, label)
	}
	total, _ := amountOf(t, totalsOf(body)["billed"], "ARS")
	if sum != total || total != 1500 {
		t.Errorf("rows add %d, total %d, want both 1500", sum, total)
	}
	if labels[0] != "Dr. Bruno" || labels[1] != "Dra. Ana" || labels[2] != "" {
		t.Errorf("labels = %v, want by name with the group without a professional last", labels)
	}

	// The filter narrows before the cut.
	_, narrowed := billingReport(t, h, owner, novemberQuery+"&groupBy=professional&professionalId="+ana.ID)
	if rows := listOf(narrowed, "rows"); len(rows) != 1 {
		t.Errorf("narrowed rows = %d, want 1", len(rows))
	}
	if amount, _ := amountOf(t, totalsOf(narrowed)["billed"], "ARS"); amount != 300 {
		t.Errorf("narrowed total = %d, want 300", amount)
	}
}

// TestTheReportCutsByMutual: a patient covered by a mutual is valued at the whole tariff
// (the mutual's share plus the copago) and falls under the mutual.
func TestTheReportCutsByMutual(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	mutual := seedMutualInsurer(t, h, tenantA, "OSDE", someTaxID(settlementTaxID))
	professional := seedProfessional(t, h, tenantA, "Dra. Mutual")
	seedMutualPrice(t, h, tenantA, professional.ID, mutual, 800000) // plus a copago of 150000
	covered := seedCoveredPatient(t, h, owner, tenantA, mutual, "Ana Mutual", "30111222")
	private := createPatient(t, h, owner, "Sofía Particular", "30555666")
	seedParticularTariff(t, h, tenantA, professional.ID, 1500000)
	for _, subject := range []string{covered, private} {
		seedAttendedSession(t, h, tenantA, subject, professional, time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC))
	}

	status, body := billingReport(t, h, owner, novemberQuery+"&groupBy=insurer")
	if status != http.StatusOK {
		t.Fatalf("report = %d, want 200: %v", status, body)
	}
	rows := listOf(body, "rows")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want the mutual and the group without one: %v", len(rows), rows)
	}
	first, _ := rows[0].(map[string]any)
	if first["label"] != "OSDE" {
		t.Fatalf("first row = %v, want OSDE", first)
	}
	if amount, _ := amountOf(t, first["pending"], "ARS"); amount != 950000 {
		t.Errorf("OSDE pending = %d, want 950000 (800000 + the copago 150000)", amount)
	}

	_, onlyPrivate := billingReport(t, h, owner, novemberQuery+"&coverage=private")
	if amount, _ := amountOf(t, totalsOf(onlyPrivate)["pending"], "ARS"); amount != 1500000 {
		t.Errorf("private pending = %d, want 1500000", amount)
	}
}

// TestAProfessionalReadsOnlyTheirOwnFigures: the scope is the role's, not the request's. Asking
// for a colleague returns the professional's own figures, never the colleague's.
func TestAProfessionalReadsOnlyTheirOwnFigures(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	mine := seedProfessional(t, h, tenantA, "Dra. Mía")
	colleague := seedProfessional(t, h, tenantA, "Dr. Colega")
	at := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	seedIssuedVoucher(h, tenantA, at, "ARS", 100, professionalPhoto(mine))
	seedIssuedVoucher(h, tenantA, at, "ARS", 900, professionalPhoto(colleague))
	seedIssuedVoucher(h, tenantA, at, "ARS", 5000, billing.Photo{}) // a loose charge is not any professional's

	token, err := h.tokens.Issue(authn.AccessClaims{Subject: mine.ID, TenantID: tenantA, Role: string(iam.RoleProfessional)})
	if err != nil {
		t.Fatalf("cannot issue a token: %v", err)
	}

	status, body := billingReport(t, h, token, novemberQuery)
	if status != http.StatusOK {
		t.Fatalf("report as professional = %d, want 200: %v", status, body)
	}
	if body["scope"] != "own" {
		t.Errorf("scope = %v, want own", body["scope"])
	}
	if amount, _ := amountOf(t, totalsOf(body)["billed"], "ARS"); amount != 100 {
		t.Errorf("own billed = %d, want 100 (not the colleague's 900, not the loose charge)", amount)
	}

	_, nosy := billingReport(t, h, token, novemberQuery+"&professionalId="+colleague.ID)
	if amount, _ := amountOf(t, totalsOf(nosy)["billed"], "ARS"); amount != 100 {
		t.Errorf("asking for the colleague returned %d, want the professional's own 100", amount)
	}
}

// TestWhoMayReadTheReport: the owner, the admin and the accountant read the practice's figures;
// the assistant and an anonymous caller read nothing.
func TestWhoMayReadTheReport(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	seedIssuedVoucher(h, tenantA, time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC), "ARS", 100, billing.Photo{})

	for _, role := range []iam.RoleCode{iam.RoleOwner, iam.RoleAdmin, iam.RoleAccountant} {
		status, body := billingReport(t, h, h.token(t, tenantA, role), novemberQuery)
		if status != http.StatusOK || body["scope"] != "practice" {
			t.Errorf("%s = %d scope %v, want 200 practice", role, status, body["scope"])
		}
	}
	if status, _ := billingReport(t, h, h.token(t, tenantA, iam.RoleAssistant), novemberQuery); status != http.StatusForbidden {
		t.Errorf("assistant = %d, want 403", status)
	}
	if status, _ := billingReport(t, h, "", novemberQuery); status != http.StatusUnauthorized {
		t.Errorf("anonymous = %d, want 401", status)
	}
}

// TestTheReportOfAPracticeNeverShowsAnotherOnes: the vouchers of the other practice are not
// added, whoever asks.
func TestTheReportOfAPracticeNeverShowsAnotherOnes(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")
	at := time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)
	seedIssuedVoucher(h, tenantA, at, "ARS", 100, billing.Photo{})
	seedIssuedVoucher(h, tenantB, at, "ARS", 9000, billing.Photo{})

	_, body := billingReport(t, h, h.token(t, tenantA, iam.RoleOwner), novemberQuery)
	if amount, _ := amountOf(t, totalsOf(body)["billed"], "ARS"); amount != 100 {
		t.Errorf("billed = %d, want only this practice's 100", amount)
	}
}

func TestTheReportRefusesARequestItCannotRead(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	for name, query := range map[string]string{
		"no range":              "",
		"reversed range":        "from=2026-11-30&to=2026-11-01",
		"impossible day":        "from=2026-02-30&to=2026-03-01",
		"range too long":        "from=2018-01-01&to=2026-12-31",
		"unknown cut":           novemberQuery + "&groupBy=day",
		"unknown coverage":      novemberQuery + "&coverage=other",
		"unknown modality":      novemberQuery + "&modality=phone",
		"professional no id":    novemberQuery + "&professionalId=x",
		"private with a mutual": novemberQuery + "&coverage=private&insurerId=" + uuid.NewString(),
	} {
		if status, body := billingReport(t, h, owner, query); status != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d, want 422: %v", name, status, body)
		}
	}
}

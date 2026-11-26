package http_test

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// fakeBillingRepository keeps vouchers per practice in memory and reads the subjects
// from the patient double, the way the adapter reads them from the patients table. The
// numbering mirrors the counter: a voucher takes the next number of its practice and a
// void never gives it back.
type fakeBillingRepository struct {
	patients *fakePatientRepository
	// clients is the payer of each patient: what the receipt stamps (S26, T73).
	clients  *fakeClientRepository
	vouchers map[string]map[billing.VoucherID]billing.Voucher
	counters map[string]int
	// sessions and professionals are the appointments and active professionals of each
	// practice; insurerNames names the mutuals a patient can be covered by.
	sessions      map[string]map[string]billing.Appointment
	professionals map[string]map[string]billing.Professional
	insurerNames  map[string]string
}

func newFakeBillingRepository(patients *fakePatientRepository, clients *fakeClientRepository) *fakeBillingRepository {
	return &fakeBillingRepository{
		patients:      patients,
		clients:       clients,
		vouchers:      map[string]map[billing.VoucherID]billing.Voucher{},
		counters:      map[string]int{},
		sessions:      map[string]map[string]billing.Appointment{},
		professionals: map[string]map[string]billing.Professional{},
		insurerNames:  map[string]string{},
	}
}

// billedBy returns the issued voucher that bills a session, or "".
func (r *fakeBillingRepository) billedBy(tenantID, appointmentID string) string {
	for id, voucher := range r.vouchers[tenantID] {
		if voucher.AppointmentID == appointmentID && voucher.Status == billing.StatusIssued {
			return string(id)
		}
	}
	return ""
}

func (r *fakeBillingRepository) PatientBilling(ctx context.Context, patientID string) (billing.PatientBilling, error) {
	subject, err := r.patients.ByID(ctx, patient.ID(patientID))
	if err != nil {
		return billing.PatientBilling{}, billing.ErrPatientNotVisible
	}
	terms := billing.PatientBilling{
		Coverage: billing.Coverage(subject.CoverageKind),
		Active:   subject.IsActive,
	}
	// Since T81 the proposed currency is read from the client row, never from a
	// copy on the patient double.
	if payer, err := r.clients.ByID(ctx, client.ID(subject.ClientID)); err == nil {
		terms.Currency = string(payer.BillingCurrency)
	}
	if subject.InsurerID != nil {
		terms.InsurerID = *subject.InsurerID
		terms.InsurerName = r.insurerNames[*subject.InsurerID]
	}
	return terms, nil
}

func (r *fakeBillingRepository) Appointment(ctx context.Context, appointmentID string) (billing.Appointment, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Appointment{}, billing.ErrTenantScopeMissing
	}
	session, found := r.sessions[tenantID][appointmentID]
	if !found {
		return billing.Appointment{}, billing.ErrAppointmentNotVisible
	}
	session.BilledVoucherID = r.billedBy(tenantID, appointmentID)
	return session, nil
}

// PatientAppointments answers the newest sessions first, the way the adapter orders them.
// UnbilledSessions mirrors the query: attended sessions inside the window that no issued
// voucher bills, oldest first, narrowed to a payer when one is asked, each row naming the
// payer of its patient. The fake holds no patient names beyond the repository, so a name
// missing there stays empty.
func (r *fakeBillingRepository) UnbilledSessions(
	ctx context.Context,
	filter port.UnbilledFilter,
) ([]billing.UnbilledSession, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}

	var matches []billing.UnbilledSession
	for _, session := range r.sessions[tenantID] {
		if session.Status != "attended" {
			continue
		}
		if session.StartsAt.Before(filter.From) || !session.StartsAt.Before(filter.To) {
			continue
		}
		if r.billedBy(tenantID, session.ID) != "" {
			continue
		}
		clientID, clientName := r.payerOf(ctx, session.PatientID)
		if filter.ClientID != "" && clientID != filter.ClientID {
			continue
		}
		matches = append(matches, billing.UnbilledSession{
			ID:               session.ID,
			PatientID:        session.PatientID,
			PatientName:      patientNameOf(r, ctx, session.PatientID),
			ClientID:         clientID,
			ClientName:       clientName,
			ProfessionalID:   session.ProfessionalID,
			ProfessionalName: session.ProfessionalName,
			Modality:         session.Modality,
			StartsAt:         session.StartsAt,
			Status:           session.Status,
		})
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].StartsAt.Before(matches[j].StartsAt) })
	if int32(len(matches)) > filter.Limit {
		matches = matches[:filter.Limit]
	}
	return matches, nil
}

// payerOf resolves the payer of a patient: its client's id and display name.
func (r *fakeBillingRepository) payerOf(ctx context.Context, patientID string) (string, string) {
	subject, err := r.patients.ByID(ctx, patient.ID(patientID))
	if err != nil || subject.ClientID == "" {
		return "", ""
	}
	payer, err := r.clients.ByID(ctx, client.ID(subject.ClientID))
	if err != nil {
		return subject.ClientID, ""
	}
	return subject.ClientID, payer.DisplayName
}

// patientNameOf reads the patient's name from the patient double.
func patientNameOf(r *fakeBillingRepository, ctx context.Context, patientID string) string {
	subject, err := r.patients.ByID(ctx, patient.ID(patientID))
	if err != nil {
		return ""
	}
	return subject.FullName
}

func (r *fakeBillingRepository) PatientAppointments(ctx context.Context, patientID string, limit int32) ([]billing.Appointment, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	var matches []billing.Appointment
	for _, session := range r.sessions[tenantID] {
		if session.PatientID != patientID {
			continue
		}
		session.BilledVoucherID = r.billedBy(tenantID, session.ID)
		matches = append(matches, session)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].StartsAt.After(matches[j].StartsAt) })
	if int32(len(matches)) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func (r *fakeBillingRepository) Professional(ctx context.Context, userID string) (billing.Professional, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Professional{}, billing.ErrTenantScopeMissing
	}
	professional, found := r.professionals[tenantID][userID]
	if !found {
		return billing.Professional{}, billing.ErrProfessionalNotVisible
	}
	return professional, nil
}

func (r *fakeBillingRepository) Professionals(ctx context.Context) ([]billing.Professional, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	var matches []billing.Professional
	for _, professional := range r.professionals[tenantID] {
		matches = append(matches, professional)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].FullName < matches[j].FullName })
	return matches, nil
}

var _ port.BillingRepository = (*fakeBillingRepository)(nil)

func (r *fakeBillingRepository) Issue(ctx context.Context, voucher billing.Voucher) (billing.Voucher, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Voucher{}, billing.ErrTenantScopeMissing
	}
	// Like the partial unique index of 0020: a session is billed by one issued voucher.
	if voucher.AppointmentID != "" && r.billedBy(tenantID, voucher.AppointmentID) != "" {
		return billing.Voucher{}, billing.ErrAppointmentAlreadyBilled
	}
	r.counters[tenantID]++
	numbered, err := voucher.AssignNumber(r.counters[tenantID])
	if err != nil {
		return billing.Voucher{}, err
	}
	if r.vouchers[tenantID] == nil {
		r.vouchers[tenantID] = map[billing.VoucherID]billing.Voucher{}
	}
	r.vouchers[tenantID][numbered.ID] = numbered
	return numbered, nil
}

func (r *fakeBillingRepository) Void(ctx context.Context, voucher billing.Voucher) (billing.Voucher, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Voucher{}, billing.ErrTenantScopeMissing
	}
	current, found := r.vouchers[tenantID][voucher.ID]
	if !found {
		return billing.Voucher{}, billing.ErrNotFound
	}
	if current.Status == billing.StatusVoided {
		return billing.Voucher{}, billing.ErrAlreadyVoided
	}
	r.vouchers[tenantID][voucher.ID] = voucher
	return voucher, nil
}

func (r *fakeBillingRepository) ByID(ctx context.Context, id billing.VoucherID) (billing.Voucher, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Voucher{}, billing.ErrTenantScopeMissing
	}
	voucher, found := r.vouchers[tenantID][id]
	if !found {
		return billing.Voucher{}, billing.ErrNotFound
	}
	return voucher, nil
}

// List answers newest first, filtered the way the adapter filters.
func (r *fakeBillingRepository) List(ctx context.Context, filter port.VoucherFilter) ([]billing.Voucher, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	var matches []billing.Voucher
	for _, voucher := range r.vouchers[tenantID] {
		if filter.Status != "" && string(voucher.Status) != filter.Status {
			continue
		}
		if filter.PatientID != "" && voucher.PatientID != filter.PatientID {
			continue
		}
		matches = append(matches, voucher)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Number > matches[j].Number })
	return matches, nil
}

func (r *fakeBillingRepository) Subject(ctx context.Context, patientID string) (billing.Subject, error) {
	subject, err := r.patients.ByID(ctx, patient.ID(patientID))
	if err != nil {
		return billing.Subject{}, billing.ErrPatientNotVisible
	}
	return r.subjectOf(ctx, subject), nil
}

// SearchSubjects matches the fragment literally, against the active patients only.
func (r *fakeBillingRepository) SearchSubjects(ctx context.Context, fragment string, limit int32) ([]billing.Subject, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	var matches []billing.Subject
	for _, subject := range r.patients.rows[tenantID] {
		if !subject.IsActive {
			continue
		}
		if !strings.Contains(strings.ToLower(subject.FullName), strings.ToLower(fragment)) {
			continue
		}
		matches = append(matches, r.subjectOf(ctx, subject))
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].FullName < matches[j].FullName })
	if int32(len(matches)) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

// subjectOf is the projection the billing port promises: identity from the patient row,
// the payer's id and the fiscal data from the billing client — the mirror on the patient
// row was dropped by 0028 (T85).
func (r *fakeBillingRepository) subjectOf(ctx context.Context, subject patient.Patient) billing.Subject {
	out := billing.Subject{
		PatientID: string(subject.ID),
		ClientID:  subject.ClientID,
		FullName:  subject.FullName,
	}
	if payer, err := r.clients.ByID(ctx, client.ID(subject.ClientID)); err == nil {
		out.FiscalCondition = payer.FiscalCondition
		out.BillingCurrency = string(payer.BillingCurrency)
		if payer.TaxID != nil {
			number := payer.TaxID.Number
			out.TaxID = &number
		}
	}
	if subject.Document != nil {
		number := subject.Document.Number
		out.DocumentNumber = &number
	}
	return out
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// issueVoucher sends an issue request and returns the answer.
func issueVoucher(t *testing.T, h *harness, token string, body map[string]any) (int, map[string]any) {
	t.Helper()
	return h.do(t, http.MethodPost, "/api/v1/billing/vouchers", token, body)
}

// issueLoose issues a voucher with no patient and returns its id and number.
func issueLoose(t *testing.T, h *harness, token, concept string) (string, int) {
	t.Helper()
	status, body := issueVoucher(t, h, token, map[string]any{
		"concept":     concept,
		"amountMinor": 150000,
		"currency":    "ARS",
	})
	if status != http.StatusCreated {
		t.Fatalf("issue %q = %d, want 201: %v", concept, status, body)
	}
	id, _ := body["id"].(string)
	number, _ := body["number"].(float64)
	return id, int(number)
}

// ─── tests ───────────────────────────────────────────────────────────────────

// TestOnlyTheAccountantAndTheOwnerIssueAndVoid is the matrix of S22 and L46 on the
// billing routes: the accountant and the owner write, the admin reads, and the clinical
// roles are refused before any handler runs.
func TestOnlyTheAccountantAndTheOwnerIssueAndVoid(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")

	accountant := h.token(t, tenantA, iam.RoleAccountant)
	owner := h.token(t, tenantA, iam.RoleOwner)
	admin := h.token(t, tenantA, iam.RoleAdmin)
	professional := h.token(t, tenantA, iam.RoleProfessional)
	assistant := h.token(t, tenantA, iam.RoleAssistant)

	draft := map[string]any{"concept": "Sesión", "amountMinor": 1000, "currency": "ARS"}

	for name, token := range map[string]string{"accountant": accountant, "owner": owner} {
		if status, body := issueVoucher(t, h, token, draft); status != http.StatusCreated {
			t.Fatalf("%s issues = %d, want 201: %v", name, status, body)
		}
	}
	for name, token := range map[string]string{"admin": admin, "professional": professional, "assistant": assistant} {
		if status, _ := issueVoucher(t, h, token, draft); status != http.StatusForbidden {
			t.Fatalf("%s issues = %d, want 403", name, status)
		}
	}

	id, _ := issueLoose(t, h, accountant, "Para anular")
	voidPath := "/api/v1/billing/vouchers/" + id + "/void"
	if status, _ := h.do(t, http.MethodPost, voidPath, admin, map[string]any{"reason": "Error"}); status != http.StatusForbidden {
		t.Fatalf("admin voids = %d, want 403: admin reads billing, it does not void", status)
	}
	if status, _ := h.do(t, http.MethodPost, voidPath, accountant, map[string]any{"reason": "Error"}); status != http.StatusOK {
		t.Fatalf("accountant voids = %d, want 200", status)
	}

	for name, token := range map[string]string{"accountant": accountant, "owner": owner, "admin": admin} {
		if status, _ := h.do(t, http.MethodGet, "/api/v1/billing/vouchers", token, nil); status != http.StatusOK {
			t.Fatalf("%s reads the history = %d, want 200", name, status)
		}
	}
	for name, token := range map[string]string{"professional": professional, "assistant": assistant} {
		if status, body := h.do(t, http.MethodGet, "/api/v1/billing/vouchers", token, nil); status != http.StatusForbidden {
			t.Fatalf("%s reads the history = %d, want 403", name, status)
		} else if _, leaked := body["items"]; leaked {
			t.Fatalf("%s received a voucher list: %v", name, body)
		}
	}
}

// TestTheBuyerAndThePracticeComeFromTheServer pins the snapshot: the buyer is the client
// that pays for the patient (S26, T73) and the practice name is the tenant's. A request
// that names another buyer is refused, because the field is not part of the contract.
//
// Issuing for a patient that has no client cannot happen through this layer: the schema of
// 0026 makes patients.client_id mandatory, so every patient of the practice has a payer.
// The domain still refuses a payerless draft (TestNewIssuedValidation, "patient without
// buyer"), which is the only answer such state could produce.
func TestTheBuyerAndThePracticeComeFromTheServer(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	status, created := h.do(t, http.MethodPost, "/api/v1/patients", owner, newPatientCreateBody(map[string]any{
		"fullName":       "Ana Pérez",
		"documentKind":   "dni",
		"documentNumber": "30111222",
		"client": map[string]any{
			"displayName":     "Estudio Pérez S.A.",
			"billingCurrency": "ARS",
			"taxIdKind":       "cuit",
			"taxIdNumber":     "20-12345678-6",
			"fiscalCondition": "monotributo",
		},
	}))
	if status != http.StatusCreated {
		t.Fatalf("create patient = %d, want 201: %v", status, created)
	}
	patientID, _ := created["id"].(string)
	clientID, _ := created["clientId"].(string)

	// The contract has no field for the buyer or the practice: naming one is refused.
	if status, body := issueVoucher(t, h, accountant, map[string]any{
		"patientId":    patientID,
		"concept":      "Sesión individual",
		"amountMinor":  200000,
		"currency":     "ARS",
		"buyerName":    "Otra persona",
		"practiceName": "Nombre inventado",
	}); status != http.StatusUnprocessableEntity {
		t.Fatalf("issue naming a buyer = %d, want 422: %v", status, body)
	}

	status, body := issueVoucher(t, h, accountant, map[string]any{
		"patientId":   patientID,
		"concept":     "Sesión individual",
		"amountMinor": 200000,
		"currency":    "ARS",
	})
	if status != http.StatusCreated {
		t.Fatalf("issue for a patient = %d, want 201: %v", status, body)
	}
	voucherID, _ := body["id"].(string)

	// The client's name is not the patient's, and its paper is a CUIT while the patient
	// only carries a DNI: this buyer line can only come from the client.
	if body["buyerName"] != "Estudio Pérez S.A." {
		t.Errorf("buyerName = %v, want the client's name: the request cannot name the buyer", body["buyerName"])
	}
	if body["buyerTaxId"] != "20-12345678-6" {
		t.Errorf("buyerTaxId = %v, want the client's tax id", body["buyerTaxId"])
	}
	if body["buyerFiscalCondition"] != "monotributo" {
		t.Errorf("buyerFiscalCondition = %v, want the client's", body["buyerFiscalCondition"])
	}
	if body["practiceName"] != "practica-a" {
		t.Errorf("practiceName = %v, want the tenant's name: the letterhead is not typed", body["practiceName"])
	}

	// The snapshot of a voucher survives an edit of its client: the issued one keeps the
	// buyer it was printed with, and only a voucher issued later stamps the new name.
	if status, _ := h.do(t, http.MethodPatch, "/api/v1/clients/"+clientID, owner, map[string]any{
		"displayName":     "Estudio Renombrado S.A.",
		"country":         "AR",
		"taxIdKind":       "cuit",
		"taxIdNumber":     "20-12345678-6",
		"fiscalCondition": "monotributo",
		"billingCurrency": "ARS",
	}); status != http.StatusOK {
		t.Fatalf("edit the client = %d, want 200", status)
	}

	status, first := h.do(t, http.MethodGet, "/api/v1/billing/vouchers/"+voucherID, accountant, nil)
	if status != http.StatusOK {
		t.Fatalf("read the old voucher = %d, want 200: %v", status, first)
	}
	if first["buyerName"] != "Estudio Pérez S.A." {
		t.Errorf("buyerName after the edit = %v, want the snapshot taken at issue", first["buyerName"])
	}

	status, second := issueVoucher(t, h, accountant, map[string]any{
		"patientId":   patientID,
		"concept":     "Sesión individual",
		"amountMinor": 200000,
		"currency":    "ARS",
	})
	if status != http.StatusCreated {
		t.Fatalf("issue after the edit = %d, want 201: %v", status, second)
	}
	if second["buyerName"] != "Estudio Renombrado S.A." {
		t.Errorf("buyerName of the new voucher = %v, want the client's name at issue", second["buyerName"])
	}
}

// TestTheCurrencyIsTheClientsBillingCurrency: a voucher for a patient is issued in the
// payer's currency, not the patient row's. The patient's row still mirrors the payer's
// data as it was when the patient was created (T72), so this test edits the client to a
// different currency and pins the difference: the old mirror would accept the old currency,
// the client rule refuses it.
func TestTheCurrencyIsTheClientsBillingCurrency(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	status, created := h.do(t, http.MethodPost, "/api/v1/patients", owner, newPatientCreateBody(map[string]any{
		"fullName":       "Tina Euro",
		"documentKind":   "dni",
		"documentNumber": "30555666",
	}))
	if status != http.StatusCreated {
		t.Fatalf("create patient = %d, want 201: %v", status, created)
	}
	patientID, _ := created["id"].(string)
	clientID, _ := created["clientId"].(string)

	status, read := h.do(t, http.MethodGet, "/api/v1/patients/"+patientID, owner, nil)
	if status != http.StatusOK {
		t.Fatalf("read the patient = %d, want 200: %v", status, read)
	}
	if read["clientId"] != clientID {
		t.Fatalf("clientId = %v, want %s", read["clientId"], clientID)
	}

	if status, issued := issueVoucher(t, h, accountant, map[string]any{
		"patientId":   patientID,
		"concept":     "Sesión individual",
		"amountMinor": 200000,
		"currency":    "ARS",
	}); status != http.StatusCreated {
		t.Fatalf("issue in the client's currency = %d, want 201: the client is still ARS: %v", status, issued)
	}

	if status, _ := h.do(t, http.MethodPatch, "/api/v1/clients/"+clientID, owner, map[string]any{
		"displayName":     "Paga en euros S.A.",
		"country":         "AR",
		"billingCurrency": "EUR",
	}); status != http.StatusOK {
		t.Fatalf("edit the client to EUR = %d, want 200", status)
	}

	// The client now bills in EUR: the ARS the patient's mirror still shows is refused.
	status, body := issueVoucher(t, h, accountant, map[string]any{
		"patientId":   patientID,
		"concept":     "Sesión individual",
		"amountMinor": 200000,
		"currency":    "ARS",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("issue in the old mirror's currency = %d, want 422: %v", status, body)
	}

	status, issued := issueVoucher(t, h, accountant, map[string]any{
		"patientId":   patientID,
		"concept":     "Sesión individual",
		"amountMinor": 200000,
		"currency":    "EUR",
	})
	if status != http.StatusCreated {
		t.Fatalf("issue in the payer's currency = %d, want 201: %v", status, issued)
	}
	if issued["currency"] != "EUR" {
		t.Errorf("currency = %v, want EUR", issued["currency"])
	}
	if issued["buyerName"] != "Paga en euros S.A." {
		t.Errorf("buyerName = %v, want the client's edited name", issued["buyerName"])
	}
}

// TestAPatientThePracticeDoesNotHoldAnswersNotFound: an invisible patient and one that
// does not exist are the same answer. The accountant cannot tell them apart.
func TestAPatientThePracticeDoesNotHoldAnswersNotFound(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	ownerB := h.token(t, tenantB, iam.RoleOwner)

	foreign := createPatient(t, h, ownerB, "Paciente de B", "30222333")

	for name, patientID := range map[string]string{"another practice's": foreign, "nonexistent": uuid.NewString()} {
		status, body := issueVoucher(t, h, accountant, map[string]any{
			"patientId":   patientID,
			"concept":     "Sesión",
			"amountMinor": 1000,
			"currency":    "ARS",
		})
		if status != http.StatusNotFound {
			t.Fatalf("issue for %s patient = %d, want 404: %v", name, status, body)
		}
	}
}

// TestNumbersAreConsecutiveAndAVoidKeepsItsNumber covers the numbering contract and the
// rules of the void: a reason is required, a second void is refused, and the number of a
// void is never handed out again.
func TestNumbersAreConsecutiveAndAVoidKeepsItsNumber(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	first, firstNumber := issueLoose(t, h, accountant, "Primera")
	_, secondNumber := issueLoose(t, h, accountant, "Segunda")
	if firstNumber != 1 || secondNumber != 2 {
		t.Fatalf("numbers = %d, %d, want 1, 2", firstNumber, secondNumber)
	}

	voidPath := "/api/v1/billing/vouchers/" + first + "/void"
	if status, _ := h.do(t, http.MethodPost, voidPath, accountant, map[string]any{"reason": "   "}); status != http.StatusUnprocessableEntity {
		t.Fatalf("void with a blank reason = %d, want 422", status)
	}
	status, body := h.do(t, http.MethodPost, voidPath, accountant, map[string]any{"reason": "Error de carga"})
	if status != http.StatusOK {
		t.Fatalf("void = %d, want 200: %v", status, body)
	}
	if body["status"] != "voided" || body["number"] != float64(1) {
		t.Fatalf("voided = status %v number %v, want voided number 1", body["status"], body["number"])
	}
	if status, _ := h.do(t, http.MethodPost, voidPath, accountant, map[string]any{"reason": "Otra vez"}); status != http.StatusConflict {
		t.Fatalf("second void = %d, want 409", status)
	}

	_, thirdNumber := issueLoose(t, h, accountant, "Tercera")
	if thirdNumber != 3 {
		t.Fatalf("after a void the next number = %d, want 3: the voided number is not reused", thirdNumber)
	}
}

// TestTheSubjectSearchIsNotARoster: the search refuses a fragment too short to be a
// search, bounds its page, and finds only active patients of the practice.
func TestTheSubjectSearchIsNotARoster(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	owner := h.token(t, tenantA, iam.RoleOwner)
	createPatient(t, h, owner, "Ana Pérez", "30111222")
	createPatient(t, h, owner, "Bruno Díaz", "31222333")

	for _, fragment := range []string{"", "An"} {
		status, body := h.do(t, http.MethodGet, "/api/v1/billing/subjects?search="+fragment, accountant, nil)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("search %q = %d, want 422: %v", fragment, status, body)
		}
	}

	status, body := h.do(t, http.MethodGet, "/api/v1/billing/subjects?search=ana", accountant, nil)
	if status != http.StatusOK {
		t.Fatalf("search = %d, want 200: %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("search matches = %d, want 1: %v", len(items), body)
	}
	match, _ := items[0].(map[string]any)
	if match["fullName"] != "Ana Pérez" {
		t.Fatalf("match = %v, want Ana Pérez", match)
	}
	if _, leaked := match["phone"]; leaked {
		t.Fatalf("a subject carried a contact field: %v", match)
	}
	if _, leaked := match["clinicalRecord"]; leaked {
		t.Fatalf("a subject carried clinical data: %v", match)
	}

	if status, _ := h.do(t, http.MethodGet, "/api/v1/billing/subjects?search=ana&limit=21", accountant, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("limit above 20 = %d, want 422", status)
	}
}

// TestTheHistoryFiltersByStateAndRefusesAnUnknownOne covers the list filters the screen
// uses: the state narrows the history, and an unknown state is refused.
func TestTheHistoryFiltersByStateAndRefusesAnUnknownOne(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	kept, _ := issueLoose(t, h, accountant, "Vigente")
	voided, _ := issueLoose(t, h, accountant, "Anulado")
	if status, _ := h.do(t, http.MethodPost, "/api/v1/billing/vouchers/"+voided+"/void", accountant, map[string]any{"reason": "Error"}); status != http.StatusOK {
		t.Fatalf("void = %d, want 200", status)
	}

	_, body := h.do(t, http.MethodGet, "/api/v1/billing/vouchers?status=voided", accountant, nil)
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("voided history = %d, want 1: %v", len(items), body)
	}
	if got, _ := items[0].(map[string]any); got["id"] == kept {
		t.Fatalf("the issued voucher showed up in the voided history")
	}

	if status, _ := h.do(t, http.MethodGet, "/api/v1/billing/vouchers?status=draft", accountant, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("unknown status = %d, want 422", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/billing/vouchers?from=ayer", accountant, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("unparseable from = %d, want 422", status)
	}
}

// TestIssuingAndVoidingLeaveAnAuditTrail: both changes are recorded with the actor, in
// the practice that made them.
func TestIssuingAndVoidingLeaveAnAuditTrail(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	id, _ := issueLoose(t, h, accountant, "Auditada")
	if status, _ := h.do(t, http.MethodPost, "/api/v1/billing/vouchers/"+id+"/void", accountant, map[string]any{"reason": "Error"}); status != http.StatusOK {
		t.Fatalf("void = %d, want 200", status)
	}

	actions := map[string]bool{}
	for _, event := range h.audit.events {
		if event.TenantID != tenantA {
			t.Fatalf("audit event for tenant %q, want %q", event.TenantID, tenantA)
		}
		actions[event.Action] = true
	}
	for _, want := range []string{"billing.issue", "billing.void"} {
		if !actions[want] {
			t.Errorf("no %q event in the trail: %v", want, h.audit.events)
		}
	}
}

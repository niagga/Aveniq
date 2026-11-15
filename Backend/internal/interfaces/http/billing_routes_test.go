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
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// fakeBillingRepository keeps vouchers per practice in memory and reads the subjects
// from the patient double, the way the adapter reads them from the patients table. The
// numbering mirrors the counter: a voucher takes the next number of its practice and a
// void never gives it back.
type fakeBillingRepository struct {
	patients *fakePatientRepository
	vouchers map[string]map[billing.VoucherID]billing.Voucher
	counters map[string]int
}

func newFakeBillingRepository(patients *fakePatientRepository) *fakeBillingRepository {
	return &fakeBillingRepository{
		patients: patients,
		vouchers: map[string]map[billing.VoucherID]billing.Voucher{},
		counters: map[string]int{},
	}
}

var _ port.BillingRepository = (*fakeBillingRepository)(nil)

func (r *fakeBillingRepository) Issue(ctx context.Context, voucher billing.Voucher) (billing.Voucher, error) {
	tenantID, ok := scopedTenant(ctx)
	if !ok {
		return billing.Voucher{}, billing.ErrTenantScopeMissing
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
	return subjectOf(subject), nil
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
		matches = append(matches, subjectOf(subject))
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].FullName < matches[j].FullName })
	if int32(len(matches)) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

// subjectOf is the projection the billing port promises: identity and fiscal data only.
func subjectOf(subject patient.Patient) billing.Subject {
	out := billing.Subject{
		PatientID:       string(subject.ID),
		FullName:        subject.FullName,
		FiscalCondition: subject.FiscalCondition,
	}
	if subject.Document != nil {
		number := subject.Document.Number
		out.DocumentNumber = &number
	}
	if subject.TaxID != nil {
		number := subject.TaxID.Number
		out.TaxID = &number
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

// TestTheBuyerAndThePracticeComeFromTheServer pins the snapshot: the buyer is the
// patient's record and the practice name is the tenant's. A request that names another
// buyer is answered with the patient's, because the field is not part of the contract.
func TestTheBuyerAndThePracticeComeFromTheServer(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	status, created := h.do(t, http.MethodPost, "/api/v1/patients", owner, map[string]any{
		"fullName":        "Ana Pérez",
		"documentKind":    "dni",
		"documentNumber":  "30111222",
		"taxIdKind":       "cuit",
		"taxIdNumber":     "20-12345678-6",
		"fiscalCondition": "monotributo",
	})
	if status != http.StatusCreated {
		t.Fatalf("create patient = %d, want 201: %v", status, created)
	}
	patientID, _ := created["id"].(string)

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
	if body["buyerName"] != "Ana Pérez" {
		t.Errorf("buyerName = %v, want the patient's name: the request cannot name the buyer", body["buyerName"])
	}
	if body["buyerTaxId"] != "20-12345678-6" {
		t.Errorf("buyerTaxId = %v, want the patient's tax id", body["buyerTaxId"])
	}
	if body["buyerFiscalCondition"] != "monotributo" {
		t.Errorf("buyerFiscalCondition = %v, want the patient's", body["buyerFiscalCondition"])
	}
	if body["practiceName"] != "practica-a" {
		t.Errorf("practiceName = %v, want the tenant's name: the letterhead is not typed", body["practiceName"])
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

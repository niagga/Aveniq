package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// seedSession puts a session of the practice in the billing double and returns its id.
func seedSession(t *testing.T, h *harness, tenantID string, session billing.Appointment) string {
	t.Helper()
	if session.ID == "" {
		session.ID = uuid.NewString()
	}
	if session.StartsAt.IsZero() {
		session.StartsAt = time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	}
	if h.vouchers.sessions[tenantID] == nil {
		h.vouchers.sessions[tenantID] = map[string]billing.Appointment{}
	}
	h.vouchers.sessions[tenantID][session.ID] = session
	return session.ID
}

// seedProfessional makes an active professional of the practice and returns its id.
func seedProfessional(t *testing.T, h *harness, tenantID, fullName string) billing.Professional {
	t.Helper()
	professional := billing.Professional{ID: uuid.NewString(), FullName: fullName}
	if h.vouchers.professionals[tenantID] == nil {
		h.vouchers.professionals[tenantID] = map[string]billing.Professional{}
	}
	h.vouchers.professionals[tenantID][professional.ID] = professional
	return professional
}

// seedParticularTariff prices a particular session of the professional from validFrom.
func seedParticularTariff(t *testing.T, h *harness, tenantID, professionalID string, patientMinor int64) {
	t.Helper()
	id := pricing.ID(uuid.NewString())
	h.tariffs.stored[id] = pricing.Tariff{
		ID:                 id,
		TenantID:           tenant.ID(tenantID),
		CreatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ProfessionalUserID: professionalID,
		Coverage:           patient.CoveragePrivate,
		Modality:           schedule.ModalityPresencial,
		Currency:           patient.CurrencyARS,
		ValidFrom:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		PatientMinor:       patientMinor,
	}
}

// TestTheProposalOfAMutualSessionIsTheTotalOfItsTariff: a session covered by a mutual is
// proposed the whole fee the tariff prices: what the mutual pays plus the copago.
func TestTheProposalOfAMutualSessionIsTheTotalOfItsTariff(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	patientID := createPatient(t, h, owner, "Luis Mutual", "30999888")
	insurerID := "0c1f3e2a-1111-4222-8333-944455556666"
	subject := h.repo.rows[tenantA][patient.ID(patientID)]
	subject.CoverageKind = patient.CoverageInsurer
	subject.InsurerID = &insurerID
	h.repo.rows[tenantA][patient.ID(patientID)] = subject
	h.vouchers.insurerNames[insurerID] = "Mutual Demo"

	professional := seedProfessional(t, h, tenantA, "Dra. Mutual")
	id := pricing.ID(uuid.NewString())
	h.tariffs.stored[id] = pricing.Tariff{
		ID:                 id,
		TenantID:           tenant.ID(tenantA),
		CreatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ProfessionalUserID: professional.ID,
		Coverage:           patient.CoverageInsurer,
		InsurerID:          &insurerID,
		Modality:           schedule.ModalityPresencial,
		Currency:           patient.CurrencyARS,
		ValidFrom:          time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		InsurerMinor:       800000,
		PatientMinor:       150000,
	}
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
	})

	status, body := h.do(t, http.MethodGet, "/api/v1/billing/subjects/"+patientID+"/sessions", accountant, nil)
	if status != http.StatusOK {
		t.Fatalf("list sessions = %d, want 200: %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("sessions = %d, want 1: %v", len(items), body)
	}
	proposal, _ := items[0].(map[string]any)["proposal"].(map[string]any)
	if proposal["amountMinor"] != float64(950000) {
		t.Errorf("proposal = %v, want 950000 (mutual 800000 + copago 150000)", proposal["amountMinor"])
	}
}

// TestASessionBillsItsPatientWithItsPhotoAndAnEditableAmount: the request names the session;
// the patient, the modality and the professional come from it, and the amount is the one sent.
func TestASessionBillsItsPatientWithItsPhotoAndAnEditableAmount(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	patientID := createPatient(t, h, owner, "Ana Pérez", "30111222")
	professional := seedProfessional(t, h, tenantA, "Dra. Demo")
	session := seedSession(t, h, tenantA, billing.Appointment{
		PatientID:        patientID,
		ProfessionalID:   professional.ID,
		ProfessionalName: professional.FullName,
		Modality:         billing.ModalityOnline,
		Status:           billing.AppointmentAttended,
	})

	status, body := issueVoucher(t, h, accountant, map[string]any{
		"appointmentId": session,
		"concept":       "Sesión individual",
		"amountMinor":   1200000,
		"currency":      "ARS",
	})
	if status != http.StatusCreated {
		t.Fatalf("issue for a session = %d, want 201: %v", status, body)
	}
	if body["patientId"] != patientID || body["appointmentId"] != session {
		t.Errorf("pair = patient %v appointment %v, want the session's patient and the session", body["patientId"], body["appointmentId"])
	}
	if body["amountMinor"] != float64(1200000) {
		t.Errorf("amountMinor = %v, want the amount the emission sent", body["amountMinor"])
	}

	photo, _ := body["photo"].(map[string]any)
	want := map[string]any{
		"coverage":         "private",
		"modality":         "online",
		"professionalId":   professional.ID,
		"professionalName": "Dra. Demo",
		"insurerId":        nil,
		"insurerName":      nil,
	}
	for field, value := range want {
		if photo[field] != value {
			t.Errorf("photo.%s = %v, want %v", field, photo[field], value)
		}
	}
}

// TestASessionIsBilledOnce: a second issued voucher for the same session answers 409.
func TestASessionIsBilledOnce(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	patientID := createPatient(t, h, owner, "Ana Pérez", "30111222")
	professional := seedProfessional(t, h, tenantA, "Dra. Demo")
	session := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
	})
	draft := map[string]any{"appointmentId": session, "concept": "Sesión", "amountMinor": 1000, "currency": "ARS"}

	if status, body := issueVoucher(t, h, accountant, draft); status != http.StatusCreated {
		t.Fatalf("first issue = %d, want 201: %v", status, body)
	}
	if status, body := issueVoucher(t, h, accountant, draft); status != http.StatusConflict {
		t.Fatalf("second issue for the same session = %d, want 409: %v", status, body)
	}
}

// TestASessionRefusesAnotherPatientACancelledOneAndAForeignCurrency: the three things a
// session cannot be billed for.
func TestASessionRefusesAnotherPatientACancelledOneAndAForeignCurrency(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	patientID := createPatient(t, h, owner, "Ana Pérez", "30111222")
	otherID := createPatient(t, h, owner, "Otro Paciente", "30222333")
	professional := seedProfessional(t, h, tenantA, "Dra. Demo")
	session := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
	})
	cancelled := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentCancelled,
	})

	cases := map[string]map[string]any{
		"another patient's session": {"patientId": otherID, "appointmentId": session, "concept": "Sesión", "amountMinor": 1000, "currency": "ARS"},
		"a cancelled session":       {"appointmentId": cancelled, "concept": "Sesión", "amountMinor": 1000, "currency": "ARS"},
		"a foreign currency":        {"appointmentId": session, "concept": "Sesión", "amountMinor": 1000, "currency": "USD"},
	}
	for name, draft := range cases {
		if status, body := issueVoucher(t, h, accountant, draft); status != http.StatusUnprocessableEntity {
			t.Fatalf("%s = %d, want 422: %v", name, status, body)
		}
	}
}

// TestTheSessionsOfAPatientCarryTheTariffProposal: the emission screen reads the amount the
// tariff proposes for each session, and no proposal where no tariff prices it. Only the
// accountant and the owner read it; the admin is refused.
func TestTheSessionsOfAPatientCarryTheTariffProposal(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	admin := h.token(t, tenantA, iam.RoleAdmin)

	patientID := createPatient(t, h, owner, "Ana Pérez", "30111222")
	professional := seedProfessional(t, h, tenantA, "Dra. Demo")
	seedParticularTariff(t, h, tenantA, professional.ID, 1500000)
	priced := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
	})
	unpriced := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityOnline, Status: billing.AppointmentAttended,
		StartsAt: time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC),
	})

	status, body := h.do(t, http.MethodGet, "/api/v1/billing/subjects/"+patientID+"/sessions", accountant, nil)
	if status != http.StatusOK {
		t.Fatalf("list sessions = %d, want 200: %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("sessions = %d, want 2: %v", len(items), body)
	}
	proposals := map[string]any{}
	for _, raw := range items {
		item := raw.(map[string]any)
		proposals[item["id"].(string)] = item["proposal"]
	}
	if got, _ := proposals[priced].(map[string]any); got == nil || got["amountMinor"] != float64(1500000) || got["currency"] != "ARS" {
		t.Errorf("proposal of the priced session = %v, want 1500000 ARS", proposals[priced])
	}
	if proposals[unpriced] != nil {
		t.Errorf("proposal of the unpriced session = %v, want null", proposals[unpriced])
	}

	if status, _ := h.do(t, http.MethodGet, "/api/v1/billing/subjects/"+patientID+"/sessions", admin, nil); status != http.StatusForbidden {
		t.Fatalf("admin lists sessions = %d, want 403", status)
	}
}

// TestALooseChargeNamesAProfessionalWhoIsOptional: a loose charge may name an active
// professional and the photo keeps it; an unknown one is refused, and a loose charge with
// no professional is still a voucher.
func TestALooseChargeNamesAProfessionalWhoIsOptional(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	professional := seedProfessional(t, h, tenantA, "Dra. Demo")

	status, body := issueVoucher(t, h, accountant, map[string]any{
		"professionalUserId": professional.ID, "concept": "Consulta suelta", "amountMinor": 1000, "currency": "ARS",
	})
	if status != http.StatusCreated {
		t.Fatalf("loose charge with a professional = %d, want 201: %v", status, body)
	}
	photo, _ := body["photo"].(map[string]any)
	if photo["professionalName"] != "Dra. Demo" || photo["coverage"] != nil {
		t.Errorf("photo = %v, want the professional and no coverage", photo)
	}

	if status, body := issueVoucher(t, h, accountant, map[string]any{
		"professionalUserId": uuid.NewString(), "concept": "Consulta suelta", "amountMinor": 1000, "currency": "ARS",
	}); status != http.StatusUnprocessableEntity {
		t.Fatalf("unknown professional = %d, want 422: %v", status, body)
	}

	if status, body := issueVoucher(t, h, accountant, map[string]any{
		"concept": "Consulta suelta", "amountMinor": 1000, "currency": "ARS",
	}); status != http.StatusCreated {
		t.Fatalf("loose charge without a professional = %d, want 201: %v", status, body)
	}
}

// TestTheProfessionalsAVoucherCanNameAreTheActiveOnes: the list the loose charge picks from.
func TestTheProfessionalsAVoucherCanNameAreTheActiveOnes(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)
	seedProfessional(t, h, tenantA, "Dra. Demo")

	status, body := h.do(t, http.MethodGet, "/api/v1/billing/professionals", accountant, nil)
	if status != http.StatusOK {
		t.Fatalf("list professionals = %d, want 200: %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("professionals = %d, want 1: %v", len(items), body)
	}
}

package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// TestTheWorklistListsOnlyWhatIsOwedAReceipt: the endpoint answers the question the billing
// screen asks - which attended sessions have no voucher - and leaves out the three ways a
// session is not owed one: it was cancelled, it never happened, or a voucher already bills it.
func TestTheWorklistListsOnlyWhatIsOwedAReceipt(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	patientID := createPatient(t, h, owner, "Ana Worklist", "30111222")
	professional := seedProfessional(t, h, tenantA, "Dra. Worklist")

	inside := time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	unbilled := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended, StartsAt: inside,
	})
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentCancelled, StartsAt: inside,
	})
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentScheduled, StartsAt: inside,
	})
	// Outside the window, even though it is attended and unbilled.
	seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
		StartsAt: inside.AddDate(0, 0, -30),
	})
	// Attended and inside the window, but a voucher already bills it.
	billed := seedSession(t, h, tenantA, billing.Appointment{
		PatientID: patientID, ProfessionalID: professional.ID, ProfessionalName: professional.FullName,
		Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended,
		StartsAt: inside.Add(time.Hour),
	})
	// Seeded straight into the double: this test is about the worklist, not about issuing.
	voucherID := billing.VoucherID(uuid.NewString())
	if h.vouchers.vouchers[tenantA] == nil {
		h.vouchers.vouchers[tenantA] = map[billing.VoucherID]billing.Voucher{}
	}
	h.vouchers.vouchers[tenantA][voucherID] = billing.Voucher{
		ID:            voucherID,
		AppointmentID: billed,
		Status:        billing.StatusIssued,
	}

	status, body := h.do(
		t,
		http.MethodGet,
		"/api/v1/billing/unbilled?from=2026-11-16T00:00:00Z&to=2026-11-23T00:00:00Z",
		accountant,
		nil,
	)
	if status != http.StatusOK {
		t.Fatalf("list unbilled = %d, want 200: %v", status, body)
	}

	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("unbilled = %d, want 1 (only the attended session with no voucher): %v", len(items), body)
	}
	row, _ := items[0].(map[string]any)
	if row["id"] != unbilled {
		t.Errorf("row id = %v, want %s", row["id"], unbilled)
	}
	if row["patientId"] != patientID {
		t.Errorf("patientId = %v, want %s", row["patientId"], patientID)
	}
	if row["professionalName"] != professional.FullName {
		t.Errorf("professionalName = %v, want %s", row["professionalName"], professional.FullName)
	}
	// The row names the payer of the patient (S26, T73): the practice created the patient
	// with its own client, whose display name is the patient's name.
	clientID, _ := row["clientId"].(string)
	if clientID == "" {
		t.Errorf("clientId = %v, want the payer of the patient: the worklist cannot filter without it", row["clientId"])
	}
	if row["clientName"] != "Ana Worklist" {
		t.Errorf("clientName = %v, want the payer the patient was created with", row["clientName"])
	}
}

// TestTheWorklistNamesTheClientAndFilterByIt: the worklist narrows to one client, and each
// row answers with the payer of its patient, which the client filter asked for.
func TestTheWorklistNamesTheClientAndFilterByIt(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	ana := createPatient(t, h, owner, "Ana Cliënte", "30111222")
	bruno := createPatient(t, h, owner, "Bruno Cliënte", "31222333")

	_, anaDetail := h.do(t, http.MethodGet, "/api/v1/patients/"+ana, owner, nil)
	anaClient, _ := anaDetail["clientId"].(string)
	if anaClient == "" {
		t.Fatalf("the patient answered no clientId: %v", anaDetail)
	}

	professional := seedProfessional(t, h, tenantA, "Dra. Worklist")
	inside := time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)
	for _, session := range []billing.Appointment{
		{PatientID: ana, ProfessionalID: professional.ID, Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended, StartsAt: inside},
		{PatientID: bruno, ProfessionalID: professional.ID, Modality: billing.ModalityPresencial, Status: billing.AppointmentAttended, StartsAt: inside.Add(time.Hour)},
	} {
		seedSession(t, h, tenantA, session)
	}

	window := "from=2026-11-16T00:00:00Z&to=2026-11-23T00:00:00Z"
	status, body := h.do(
		t,
		http.MethodGet,
		"/api/v1/billing/unbilled?"+window+"&clientId="+anaClient,
		accountant,
		nil,
	)
	if status != http.StatusOK {
		t.Fatalf("list unbilled by client = %d, want 200: %v", status, body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("unbilled of one client = %d, want 1: %v", len(items), body)
	}
	row, _ := items[0].(map[string]any)
	if row["patientId"] != ana {
		t.Errorf("row = %v, want only the payer's own patient", row)
	}
	if row["clientId"] != anaClient {
		t.Errorf("clientId = %v, want %s", row["clientId"], anaClient)
	}
	if row["clientName"] != "Ana Cliënte" {
		t.Errorf("clientName = %v, want the payer's display name", row["clientName"])
	}
}

// TestTheWorklistNeedsTheBillingPermission: the clinical roles hold no billing:read, and the
// route refuses them the worklist like the rest of the history.
func TestTheWorklistNeedsTheBillingPermission(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	professional := h.token(t, tenantA, iam.RoleProfessional)

	status, _ := h.do(
		t,
		http.MethodGet,
		"/api/v1/billing/unbilled?from=2026-11-16T00:00:00Z&to=2026-11-23T00:00:00Z",
		professional,
		nil,
	)
	if status != http.StatusForbidden {
		t.Fatalf("professional list unbilled = %d, want 403", status)
	}
}

// TestTheWorklistRefusesAnInvertedWindow: a range that ends before it starts is a client mistake,
// and answering an empty list would hide it.
func TestTheWorklistRefusesAnInvertedWindow(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	status, body := h.do(
		t,
		http.MethodGet,
		"/api/v1/billing/unbilled?from=2026-11-23T00:00:00Z&to=2026-11-16T00:00:00Z",
		accountant,
		nil,
	)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("inverted window = %d, want 422: %v", status, body)
	}
}

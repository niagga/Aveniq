package http_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// An appointment is one patient's: a voucher that names an appointment must name its patient,
// and the API says so with a 422 before the database is asked anything.
func TestAnAppointmentNeedsItsPatient(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	status, body := issueVoucher(t, h, accountant, map[string]any{
		"appointmentId": uuid.NewString(),
		"concept":       "Sesión",
		"amountMinor":   1000,
		"currency":      "ARS",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("appointment without a patient = %d, want 422: %v", status, body)
	}
}

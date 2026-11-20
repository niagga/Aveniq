package http_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// A voucher names a session of the practice. A session the practice does not hold answers
// 404, the same as one that does not exist: another practice's session is not visible.
func TestAVoucherNamesASessionOfThePractice(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	status, body := issueVoucher(t, h, accountant, map[string]any{
		"appointmentId": uuid.NewString(),
		"concept":       "Sesión",
		"amountMinor":   1000,
		"currency":      "ARS",
	})
	if status != http.StatusNotFound {
		t.Fatalf("appointment the practice does not hold = %d, want 404: %v", status, body)
	}
}

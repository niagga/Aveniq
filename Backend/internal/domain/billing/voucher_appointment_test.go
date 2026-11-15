package billing_test

import (
	"errors"
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

// An appointment is one patient's. A voucher that names an appointment without naming its
// patient is refused before it reaches the database.
func TestAnAppointmentNeedsItsPatient(t *testing.T) {
	t.Run("an appointment without a patient is refused", func(t *testing.T) {
		draft := validDraft()
		draft.AppointmentID = "2b9c1f4e-0000-4000-8000-000000000001"

		_, err := billing.NewIssued(billing.VoucherID("v-1"), "tenant-1", "actor-1", draft, now)

		var validation *billing.ValidationError
		if !errors.As(err, &validation) || validation.Field != "appointment_id" {
			t.Fatalf("error = %v, want a validation error on appointment_id", err)
		}
	})

	t.Run("an appointment with its patient is accepted", func(t *testing.T) {
		name := "Ana Pérez"
		draft := validDraft()
		draft.PatientID = "3c8d2e5f-0000-4000-8000-000000000002"
		draft.AppointmentID = "2b9c1f4e-0000-4000-8000-000000000001"
		draft.BuyerName = &name

		voucher, err := billing.NewIssued(billing.VoucherID("v-2"), "tenant-1", "actor-1", draft, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if voucher.AppointmentID != draft.AppointmentID || voucher.PatientID != draft.PatientID {
			t.Fatalf("pair = patient %q appointment %q, want the draft's", voucher.PatientID, voucher.AppointmentID)
		}
	})
}

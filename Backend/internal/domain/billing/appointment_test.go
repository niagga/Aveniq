package billing_test

import (
	"errors"
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

func session(status string) billing.Appointment {
	return billing.Appointment{
		ID:               "2b9c1f4e-0000-4000-8000-000000000001",
		PatientID:        "3c8d2e5f-0000-4000-8000-000000000002",
		ProfessionalID:   "5d1a7c20-0000-4000-8000-000000000003",
		ProfessionalName: "Demo Profesional",
		Modality:         billing.ModalityPresencial,
		Status:           status,
	}
}

func TestASessionIsBilledForItsOwnPatient(t *testing.T) {
	appointment := session(billing.AppointmentAttended)

	if err := appointment.CheckBillable(appointment.PatientID); err != nil {
		t.Fatalf("CheckBillable(own patient) = %v, want nil", err)
	}
	if err := appointment.CheckBillable(""); err != nil {
		t.Fatalf("CheckBillable(no patient named) = %v, want nil: the patient comes from the session", err)
	}

	err := appointment.CheckBillable("9e0f0000-0000-4000-8000-000000000009")
	var validation *billing.ValidationError
	if !errors.As(err, &validation) || validation.Field != "patient_id" {
		t.Fatalf("CheckBillable(other patient) = %v, want a validation error on patient_id", err)
	}
}

func TestACancelledSessionIsNotBilled(t *testing.T) {
	err := session(billing.AppointmentCancelled).CheckBillable("")
	var validation *billing.ValidationError
	if !errors.As(err, &validation) || validation.Field != "appointment_id" {
		t.Fatalf("CheckBillable(cancelled) = %v, want a validation error on appointment_id", err)
	}
}

func TestASessionIsBilledOnce(t *testing.T) {
	appointment := session(billing.AppointmentAttended)
	appointment.BilledVoucherID = "7a6b0000-0000-4000-8000-000000000007"

	if err := appointment.CheckBillable(""); !errors.Is(err, billing.ErrAppointmentAlreadyBilled) {
		t.Fatalf("CheckBillable(billed) = %v, want ErrAppointmentAlreadyBilled", err)
	}
}

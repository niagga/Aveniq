package billing

import "time"

// The statuses of a session as billing reads them. A cancelled session is not billed.
const (
	AppointmentScheduled = "scheduled"
	AppointmentAttended  = "attended"
	AppointmentCancelled = "cancelled"
)

// Appointment is a session as billing reads it: whose it is, when it was, with whom, in which
// modality, and whether an issued voucher already bills it. Every session has a patient.
type Appointment struct {
	ID               string
	PatientID        string
	ProfessionalID   string
	ProfessionalName string
	Modality         string
	StartsAt         time.Time
	Status           string
	// BilledVoucherID is the issued voucher that bills the session, or "" when none does.
	BilledVoucherID string
}

// CheckBillable says whether a voucher may bill this session for the given patient. A voucher
// that names a patient must name the session's own patient; a session is billed once, and a
// cancelled session is not billed at all.
func (a Appointment) CheckBillable(patientID string) error {
	if patientID != "" && patientID != a.PatientID {
		return &ValidationError{Field: "patient_id", Message: "must be the patient of the appointment"}
	}
	if a.Status == AppointmentCancelled {
		return &ValidationError{Field: "appointment_id", Message: "a cancelled session is not billed"}
	}
	if a.BilledVoucherID != "" {
		return ErrAppointmentAlreadyBilled
	}
	return nil
}

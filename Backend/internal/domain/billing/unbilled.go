package billing

import "time"

// UnbilledSession is an attended session that no issued voucher bills: the unit of the billing
// worklist.
//
// It is not an Appointment because the worklist names the patient, and the emission screen
// already knows whose sessions it is asking for. A voided voucher bills nothing, so its session
// is an UnbilledSession again; a cancelled or still-scheduled one never is, because nobody owes
// a receipt for it.
type UnbilledSession struct {
	ID          string
	PatientID   string
	PatientName string
	// ClientID and ClientName name the payer of the patient (S26, T73): who a receipt issued
	// today would bill.
	ClientID         string
	ClientName       string
	ProfessionalID   string
	ProfessionalName string
	Modality         string
	StartsAt         time.Time
	Status           string
}

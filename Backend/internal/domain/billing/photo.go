package billing

import "strings"

// Coverage is how a patient was covered when a voucher was issued.
type Coverage string

const (
	// CoveragePrivate is a particular patient: the practice is paid in full.
	CoveragePrivate Coverage = "private"
	// CoverageInsurer is a patient covered by a mutual: the voucher is the patient's share.
	CoverageInsurer Coverage = "insurer"
)

// The modalities of a session, the same words the schedule uses.
const (
	ModalityOnline     = "online"
	ModalityPresencial = "presencial"
)

// Photo is what the practice knew when it issued a voucher: how the patient was covered, in
// which modality the session happened, and which professional gave it. It is a snapshot, like
// the buyer: a later change to the patient, the mutual or the professional does not rewrite it.
//
// Every field is optional on the type. Validate decides what a given voucher must carry: a
// voucher for a patient carries the coverage, a session carries the modality and the
// professional, and a loose charge carries none of the patient's data.
type Photo struct {
	Coverage         Coverage
	InsurerID        string
	InsurerName      string
	Modality         string
	ProfessionalID   string
	ProfessionalName string
}

// normalized trims the text fields. It keeps everything else as given, so that validate can
// refuse a field that does not belong: a particular voucher that names an insurer is wrong,
// not something to tidy away.
func (p Photo) normalized() Photo {
	return Photo{
		Coverage:         Coverage(strings.TrimSpace(string(p.Coverage))),
		InsurerID:        strings.TrimSpace(p.InsurerID),
		InsurerName:      strings.TrimSpace(p.InsurerName),
		Modality:         strings.TrimSpace(p.Modality),
		ProfessionalID:   strings.TrimSpace(p.ProfessionalID),
		ProfessionalName: strings.TrimSpace(p.ProfessionalName),
	}
}

// validate checks the photo against what the voucher names: whether it has a patient and
// whether it bills an appointment. It expects a normalized photo.
func (p Photo) validate(hasPatient, hasAppointment bool) error {
	if !hasPatient && p.Coverage != "" {
		return &ValidationError{Field: "coverage", Message: "a loose charge has no coverage"}
	}
	if hasPatient && p.Coverage == "" {
		return &ValidationError{Field: "coverage", Message: "a voucher for a patient carries the coverage"}
	}

	switch p.Coverage {
	case "":
		// A loose charge: nothing about the coverage to check.
	case CoveragePrivate:
		if p.InsurerID != "" || p.InsurerName != "" {
			return &ValidationError{Field: "insurer_id", Message: "only a voucher covered by a mutual names one"}
		}
	case CoverageInsurer:
		if p.InsurerID == "" {
			return &ValidationError{Field: "insurer_id", Message: "must not be empty for a voucher covered by a mutual"}
		}
		if p.InsurerName == "" {
			return &ValidationError{Field: "insurer_name", Message: "must not be blank for a voucher covered by a mutual"}
		}
	default:
		return &ValidationError{Field: "coverage", Message: "must be private or insurer"}
	}

	if p.Modality != "" && p.Modality != ModalityOnline && p.Modality != ModalityPresencial {
		return &ValidationError{Field: "modality", Message: "must be online or presencial"}
	}
	if hasAppointment && p.Modality == "" {
		return &ValidationError{Field: "modality", Message: "a session carries the modality it happened in"}
	}

	if (p.ProfessionalID == "") != (p.ProfessionalName == "") {
		return &ValidationError{Field: "professional_user_id", Message: "a professional needs both its identity and its name"}
	}
	if hasAppointment && p.ProfessionalID == "" {
		return &ValidationError{Field: "professional_user_id", Message: "a session carries the professional who gave it"}
	}
	return nil
}

// PatientBilling is how a patient is billed: the coverage, the mutual if there is one, and the
// currency the patient's vouchers are issued in.
type PatientBilling struct {
	Coverage    Coverage
	InsurerID   string
	InsurerName string
	Currency    string
	// Active is false for a patient that was deactivated: a past session can still owe a voucher.
	Active bool
}

// Professional is an active professional of the practice, the only role a voucher may name.
type Professional struct {
	ID       string
	FullName string
}

// Package care holds the instance-level rules of a practice: which patients a
// professional may see, treat and read the clinical record of, and who may book
// with whom.
//
// The permission matrix (domain/iam) answers "may this role use this verb at all".
// These rules answer the next question, "on this patient, or this appointment". A
// role that holds clinical:read:own still cannot read the record of a patient it
// does not treat and has no grant for, so the matrix alone is never enough.
//
// Every function here is pure: the caller loads the patient and says whether the
// viewer holds a live grant, so the rules are decided in one place and tested
// without a database.
package care

import (
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// Viewer is the verified caller a rule is decided for.
type Viewer struct {
	UserID string
	Role   iam.RoleCode
}

// isProfessional reports whether the viewer is subject to the per-patient rules.
// The other roles see the whole practice's patients and book with any of them; the
// clinical record is the exception, and it is closed to every role but the treating
// professional and the granted ones.
func (v Viewer) isProfessional() bool {
	return v.Role == iam.RoleProfessional
}

// Treats reports whether the viewer is the treating professional of the patient.
//
// A patient with no treating professional is treated by nobody: a professional
// does not see it, so it cannot be treated by one until an owner or admin assigns
// one.
func Treats(v Viewer, subject patient.Patient) bool {
	return v.isProfessional() &&
		subject.TreatingProfessionalID != nil &&
		*subject.TreatingProfessionalID == v.UserID
}

// SeesPatient reports whether the patient appears to the viewer at all. A professional
// sees the patients it treats and the ones it was granted; the rest of the practice
// sees all of them. The caller passes granted only when a live grant exists.
func SeesPatient(v Viewer, subject patient.Patient, granted bool) bool {
	if !v.isProfessional() {
		return true
	}
	return Treats(v, subject) || granted
}

// EditsPatient reports whether the viewer may change the patient's file or
// deactivate it. A professional edits only its own patients; a grant is read access
// to the file and never an edit.
func EditsPatient(v Viewer, subject patient.Patient) bool {
	return !v.isProfessional() || Treats(v, subject)
}

// ReadsClinical reports whether the viewer may read the clinical record of the
// patient. Owner, admin and assistant never read it: the owner holds every
// permission by definition, and the clinical record is still closed to them unless
// they treat the patient or were granted it.
func ReadsClinical(v Viewer, subject patient.Patient, granted bool) bool {
	return v.isProfessional() && (Treats(v, subject) || granted)
}

// WritesClinical reports whether the viewer may write the clinical record. A granted
// professional reads it and does not write it: writing belongs to the treating
// professional.
func WritesClinical(v Viewer, subject patient.Patient) bool {
	return Treats(v, subject)
}

// Books reports whether the viewer may book an appointment for the patient. Owner,
// admin and assistant book with any patient of the practice; a professional books
// only with the patients it treats, never with one that was only granted to it.
func Books(v Viewer, subject patient.Patient) bool {
	return !v.isProfessional() || Treats(v, subject)
}

// ManagesAppointment reports whether the viewer may attend, cancel, or see the
// appointment of the given professional. A professional works only on its own
// appointments; the rest of the practice works on all of them.
func ManagesAppointment(v Viewer, professionalUserID string) bool {
	return !v.isProfessional() || professionalUserID == v.UserID
}

// GrantsClinical reports whether the viewer may give a clinical grant over the patient.
// Only the treating professional gives one.
func GrantsClinical(v Viewer, subject patient.Patient) bool {
	return Treats(v, subject)
}

// ManagesGrants reports whether the viewer may list and withdraw the grants over the
// patient. That is the treating professional, and the owner: a grant must not outlive
// a treating professional who left and was replaced, so the practice keeps the hand on
// it.
func ManagesGrants(v Viewer, subject patient.Patient) bool {
	return Treats(v, subject) || v.Role == iam.RoleOwner
}

// BecomesTreatingOnCreate reports whether a patient the viewer registers is treated by
// the viewer at once. Only a professional registering a patient becomes its treating
// professional; the rest of the practice registers without one.
func BecomesTreatingOnCreate(v Viewer) bool {
	return v.isProfessional()
}

// OwnOnly reports whether the viewer's listings are narrowed to what the viewer treats or
// works on: the patients it treats or was granted, and its own appointments. Only the
// professional's listings are narrowed.
func OwnOnly(v Viewer) bool {
	return v.isProfessional()
}

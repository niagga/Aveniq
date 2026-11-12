// Package clinical holds the clinical record: the stable frame of a treatment and
// the story of its encounters.
//
// It is pure domain code. It knows nothing about SQL, HTTP or the country catalogs,
// and it deliberately does not import the patient package: the record references a
// patient by identifier, and validating that identifier belongs to the caller that
// can see both.
//
// The two aggregates live together because they are read together and their
// invariants are entangled — a note without a patient is as meaningless as a
// treatment frame without one — but they are separate types because they have
// different lifecycles: the frame is edited, the notes are appended.
package clinical

import (
	"errors"
	"fmt"
)

// Sentinel errors the delivery and infrastructure layers map onto transport codes.
var (
	// ErrNotFound reports that no record matches the requested patient.
	ErrNotFound = errors.New("clinical: not found")
	// ErrPatientNotVisible reports that the patient the row references is not
	// visible in the current tenant scope. The database refuses that write through a
	// composite foreign key; this is the readable form of the refusal.
	ErrPatientNotVisible = errors.New("clinical: the patient is not visible in this tenant scope")
	// ErrTenantScopeMissing reports an operation attempted without an open tenant
	// transaction.
	ErrTenantScopeMissing = errors.New("clinical: no tenant context: the operation must run inside WithTenant")
	// ErrInsufficientPrivilege reports a statement the database role may not run.
	ErrInsufficientPrivilege = errors.New("clinical: the database role is not allowed to run this statement")
)

// ValidationError reports a violated invariant and names the offending field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("clinical: invalid %s: %s", e.Field, e.Message)
}

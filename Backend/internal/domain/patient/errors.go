// Package patient holds the patient aggregate: the practice's client record,
// personal and fiscal data included.
//
// It is pure domain code: no SQL, no HTTP, no knowledge of which countries exist.
// A country-specific rule arrives as data (`DocumentKind`) and is applied here;
// the domain never asks "is this Argentina?".
package patient

import (
	"errors"
	"fmt"
)

// Sentinel errors the delivery and infrastructure layers map onto transport codes.
var (
	// ErrNotFound reports that no patient matches the requested identity.
	ErrNotFound = errors.New("patient: not found")
	// ErrDocumentInvalid reports a document that does not match its country's rules.
	ErrDocumentInvalid = errors.New("patient: invalid document")
	// ErrDocumentTaken reports a per-tenant uniqueness conflict on the document.
	ErrDocumentTaken = errors.New("patient: document already taken in this tenant")
	// ErrInvalidTransition reports a lifecycle change the aggregate does not allow.
	ErrInvalidTransition = errors.New("patient: invalid transition")
	// ErrTenantScopeMissing reports an operation attempted without an open tenant
	// transaction. The adapter returns it before any statement runs, so a missing
	// scope is loud and never disguised as an empty result.
	ErrTenantScopeMissing = errors.New("patient: no tenant context: the operation must run inside WithTenant")
	// ErrInsufficientPrivilege reports a statement the database role may not run.
	// It is deliberately distinct from ErrTenantScopeMissing: a missing grant is an
	// operational problem, and reporting it as "no tenant context" sends whoever
	// reads the logs after the wrong bug.
	ErrInsufficientPrivilege = errors.New("patient: the database role is not allowed to run this statement")
)

// ValidationError reports a violated invariant and names the offending field, so
// the API can answer 422 with something a form can point at.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("patient: invalid %s: %s", e.Field, e.Message)
}

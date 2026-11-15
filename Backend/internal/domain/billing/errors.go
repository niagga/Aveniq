// Package billing holds the internal voucher: what a practice records to keep
// its own accounts. These vouchers do not go to the tax authority and are not
// sent anywhere (S7). Electronic invoicing is a later decision.
//
// It is pure domain code. It knows nothing about SQL or HTTP. The patient and
// the appointment are referenced by identifier; whether they belong to this
// practice is a storage invariant (composite foreign keys in 0014).
package billing

import (
	"errors"
	"fmt"
)

// Sentinel errors the delivery and infrastructure layers map onto transport codes.
var (
	// ErrNotFound reports that no voucher matches the requested identity.
	ErrNotFound = errors.New("billing: not found")
	// ErrAlreadyVoided reports a second void on a voucher that is already final.
	ErrAlreadyVoided = errors.New("billing: the voucher is already voided")
	// ErrImmutable reports a change the schema (and the aggregate) refuse: an
	// issued voucher does not edit, and a voided one does not change again.
	ErrImmutable = errors.New("billing: an issued voucher is immutable except for its void")
	// ErrPatientNotVisible reports a patient UUID that is not in this tenant.
	ErrPatientNotVisible = errors.New("billing: the patient is not visible in this tenant scope")
	// ErrAppointmentNotVisible reports an appointment that the voucher's patient does not hold
	// in this tenant: another practice's, another patient's, or nonexistent. They are one answer
	// on purpose, so a voucher cannot be used to probe which appointments exist.
	ErrAppointmentNotVisible = errors.New("billing: the appointment is not visible in this tenant scope")
	// ErrActorNotVisible reports an issuer or voider UUID that is not in this tenant.
	ErrActorNotVisible = errors.New("billing: the actor is not visible in this tenant scope")
	// ErrTenantScopeMissing reports an operation attempted without WithTenant.
	ErrTenantScopeMissing = errors.New("billing: no tenant context: the operation must run inside WithTenant")
)

// ValidationError reports a violated invariant and names the offending field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("billing: invalid %s: %s", e.Field, e.Message)
}

package schedule

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound reports that no appointment or series matches the identity.
	ErrNotFound = errors.New("schedule: not found")
	// ErrOverlap reports two live appointments of the same professional colliding.
	ErrOverlap = errors.New("schedule: the professional already has an appointment in that interval")
	// ErrInvalidTransition reports a status change the aggregate does not allow.
	ErrInvalidTransition = errors.New("schedule: invalid status transition")
	// ErrPatientNotVisible reports a patient UUID that is not in this tenant.
	ErrPatientNotVisible = errors.New("schedule: the patient is not visible in this tenant scope")
	// ErrProfessionalNotVisible reports a user UUID that is not in this tenant.
	ErrProfessionalNotVisible = errors.New("schedule: the professional is not visible in this tenant scope")
	// ErrTenantScopeMissing reports an operation attempted without WithTenant.
	ErrTenantScopeMissing = errors.New("schedule: no tenant context: the operation must run inside WithTenant")
)

// ValidationError reports a violated invariant and names the offending field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("schedule: invalid %s: %s", e.Field, e.Message)
}

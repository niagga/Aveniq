package care

import (
	"errors"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// Grant is the treating professional's decision to let another professional read
// one patient's file and clinical record.
//
// A grant is a row that outlives its revocation: the audit trail must show who was
// allowed to read what and when, so revoking sets RevokedAt and keeps the row.
type Grant struct {
	ID                 string
	TenantID           tenant.ID
	PatientID          string
	ProfessionalUserID string
	GrantedBy          string
	GrantedAt          time.Time
	RevokedAt          *time.Time
}

// Live reports whether the grant currently allows the read.
func (g Grant) Live() bool {
	return g.RevokedAt == nil
}

// NewGrant gives a live grant over the patient to another professional. The rule
// that the granter treats the patient is checked here, so no caller can grant over
// a patient it does not treat even if it reaches the use case.
func NewGrant(id string, subject patient.Patient, granter Viewer, professionalUserID string, now time.Time) (Grant, error) {
	if strings.TrimSpace(id) == "" {
		return Grant{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if strings.TrimSpace(professionalUserID) == "" {
		return Grant{}, &ValidationError{Field: "professional_user_id", Message: "must not be empty"}
	}
	if !GrantsClinical(granter, subject) {
		return Grant{}, ErrForbidden
	}
	if professionalUserID == granter.UserID {
		return Grant{}, &ValidationError{Field: "professional_user_id", Message: "the treating professional already has the record"}
	}
	return Grant{
		ID:                 id,
		TenantID:           subject.TenantID,
		PatientID:          string(subject.ID),
		ProfessionalUserID: professionalUserID,
		GrantedBy:          granter.UserID,
		GrantedAt:          now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Revoke withdraws the grant. It is idempotent on purpose: revoking a grant that is
// already revoked returns it unchanged, so a double click does not turn into an error.
func (g Grant) Revoke(revoker Viewer, subject patient.Patient, now time.Time) (Grant, error) {
	if !ManagesGrants(revoker, subject) {
		return Grant{}, ErrForbidden
	}
	if !g.Live() {
		return g, nil
	}
	at := now.UTC().Truncate(time.Microsecond)
	if at.Before(g.GrantedAt) {
		at = g.GrantedAt
	}
	g.RevokedAt = &at
	return g, nil
}

// ValidationError names the field a grant refused, the same shape the other aggregates
// use so the HTTP layer maps it to 422 with the field in the message.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "care: " + e.Field + " " + e.Message
}

// ErrForbidden reports that the caller's role or relationship to the patient does not
// allow the action. It is distinct from ErrGrantNotFound: a caller who may not act
// learns that, and a caller who is not allowed to see the patient never gets here.
var ErrForbidden = errors.New("care: the caller may not act on this record")

// ErrTenantScopeMissing reports a repository call made outside WithTenant. It is a
// programming error, not a refusal, so the HTTP layer answers it as a server error.
var ErrTenantScopeMissing = errors.New("care: no tenant context: the operation must run inside WithTenant")

// ErrGrantNotFound reports that no grant with that identity exists in the practice.
var ErrGrantNotFound = errors.New("care: the clinical grant does not exist")

// ErrGrantAlreadyLive reports that the professional already holds a live grant over
// the patient. The database enforces it with a partial unique index.
var ErrGrantAlreadyLive = errors.New("care: the professional already holds a live grant on this patient")

// ErrNotProfessional reports that the user chosen as the treating professional does
// not hold the professional role.
var ErrNotProfessional = errors.New("care: the treating professional must hold the professional role")

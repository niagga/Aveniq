// Package iam holds the identity and access management aggregates: users,
// roles and sessions. Roles, users and sessions are coupled by the permission
// matrix, so they deliberately share one package instead of one package per
// table — splitting them would create import cycles.
//
// It is pure domain code: it imports nothing outside the standard library (the
// tenant package is a sibling domain) and knows nothing about HTTP, SQL, pgx or
// hashing. Argon2id and token generation belong to the auth layer, not here.
package iam

import (
	"errors"
	"fmt"
)

// Sentinel errors the delivery and infrastructure layers map onto transport codes.
var (
	// ErrNotFound reports that no user or session matches the requested identity.
	ErrNotFound = errors.New("iam: not found")
	// ErrEmailTaken reports a per-tenant uniqueness conflict on the email. The
	// same email may belong to different tenants, but only once per tenant.
	ErrEmailTaken = errors.New("iam: email already taken in this tenant")
	// ErrInvalidTransition reports a lifecycle change the aggregates do not allow.
	ErrInvalidTransition = errors.New("iam: invalid transition")
	// ErrTenantScopeMissing reports a write attempted outside WithTenant. Row-level
	// security fails closed (SQLSTATE 42501) when app.tenant_id was never set.
	ErrTenantScopeMissing = errors.New("iam: no tenant context: the operation must run inside WithTenant")
)

// ValidationError reports a violated invariant and names the offending field so
// the API can return a useful, machine-readable error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("iam: invalid %s: %s", e.Field, e.Message)
}

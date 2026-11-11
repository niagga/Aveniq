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
	// ErrTenantScopeMissing reports an operation attempted without an open tenant
	// transaction. It is raised by the adapter before any statement runs, so it
	// never depends on what row-level security happens to return.
	ErrTenantScopeMissing = errors.New("iam: no tenant context: the operation must run inside WithTenant")
	// ErrInsufficientPrivilege reports that the database refused a statement the
	// role is not allowed to run. It is deliberately distinct from
	// ErrTenantScopeMissing: a REVOKE or a missing grant is an operational
	// problem, and reporting it as "no tenant context" would send whoever reads
	// the logs looking in the wrong place.
	ErrInsufficientPrivilege = errors.New("iam: the database role is not allowed to run this statement")
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

// Package tenant holds the tenant aggregate: the isolation root of the platform.
// Every business record belongs to exactly one tenant, so this package defines
// what a valid tenant is and which lifecycle transitions are legal.
//
// It is pure domain code: it imports nothing outside the standard library and
// knows nothing about HTTP, SQL or Redis.
package tenant

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ID is the opaque identity of a tenant.
type ID string

// Status is the lifecycle state of a tenant.
type Status string

const (
	// StatusTrial is the state of a freshly created tenant: isolated and usable,
	// but not yet a paying customer.
	StatusTrial Status = "trial"
	// StatusActive is the state of a tenant whose subscription is current.
	StatusActive Status = "active"
	// StatusSuspended blocks access without destroying any data.
	StatusSuspended Status = "suspended"
)

// Slug length bounds. A slug becomes a DNS label (slug.domain), so it must also
// satisfy the 63-character DNS label limit.
const (
	minSlugLength = 3
	maxSlugLength = 63
	maxNameLength = 120

	// DefaultTimezone is the civil zone of the product's first practices.
	DefaultTimezone = "America/Argentina/Buenos_Aires"

	// TrialDuration is how long a self-registered practice stays in trial. The end is
	// stored on the tenant when it is created, so changing this constant does not
	// move the end of practices already registered.
	TrialDuration = 14 * 24 * time.Hour
)

// slugPattern enforces lowercase letters and digits separated by single inner
// hyphens: no leading or trailing hyphen, no consecutive hyphens, no underscores.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Sentinel errors the delivery and infrastructure layers map onto transport codes.
var (
	// ErrNotFound reports that no tenant matches the requested identity.
	ErrNotFound = errors.New("tenant: not found")
	// ErrSlugTaken reports a uniqueness conflict on the slug.
	ErrSlugTaken = errors.New("tenant: slug already taken")
	// ErrInvalidTransition reports a lifecycle change the aggregate does not allow.
	ErrInvalidTransition = errors.New("tenant: invalid status transition")
)

// ValidationError reports a violated invariant and names the offending field so
// the API can return a useful, machine-readable error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("tenant: invalid %s: %s", e.Field, e.Message)
}

// Tenant is the aggregate root.
type Tenant struct {
	ID          ID
	Slug        string
	Name        string
	Status      Status
	Timezone    string
	CreatedAt   time.Time
	TrialEndsAt time.Time
}

// New builds a brand-new tenant. It is the only constructor that accepts
// unvalidated input: every creation invariant lives here.
func New(id ID, slug, name string, now time.Time) (Tenant, error) {
	if id == "" {
		return Tenant{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}

	normalizedSlug, err := NormalizeSlug(slug)
	if err != nil {
		return Tenant{}, err
	}

	normalizedName := strings.TrimSpace(name)
	switch {
	case normalizedName == "":
		return Tenant{}, &ValidationError{Field: "name", Message: "must not be empty"}
	case len([]rune(normalizedName)) > maxNameLength:
		return Tenant{}, &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("must be at most %d characters", maxNameLength),
		}
	}

	created := now.UTC()
	return Tenant{
		ID:          id,
		Slug:        normalizedSlug,
		Name:        normalizedName,
		Status:      StatusTrial,
		Timezone:    DefaultTimezone,
		CreatedAt:   created,
		TrialEndsAt: created.Add(TrialDuration),
	}, nil
}

// Reconstitute rebuilds a tenant from persisted state. Persistence adapters must
// use this instead of the zero value or a struct literal: it keeps a single gate
// for the invariants that must hold for stored data too.
func Reconstitute(id ID, slug, name string, status Status, timezone string, createdAt, trialEndsAt time.Time) (Tenant, error) {
	if id == "" {
		return Tenant{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if !status.Valid() {
		return Tenant{}, &ValidationError{
			Field:   "status",
			Message: fmt.Sprintf("unknown status %q", status),
		}
	}
	if slug == "" || !slugPattern.MatchString(slug) {
		return Tenant{}, &ValidationError{Field: "slug", Message: "stored slug is not canonical"}
	}
	if timezone == "" {
		timezone = DefaultTimezone
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return Tenant{}, &ValidationError{Field: "timezone", Message: "must be a valid IANA name"}
	}
	if !trialEndsAt.After(createdAt) {
		return Tenant{}, &ValidationError{Field: "trial_ends_at", Message: "must be after the creation"}
	}

	return Tenant{
		ID:          id,
		Slug:        slug,
		Name:        name,
		Status:      status,
		Timezone:    timezone,
		CreatedAt:   createdAt.UTC(),
		TrialEndsAt: trialEndsAt.UTC(),
	}, nil
}

// NormalizeSlug lowercases, trims and validates a slug. It is exported because
// the API layer needs to reject invalid input before it reaches persistence.
func NormalizeSlug(raw string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(raw))

	switch {
	case slug == "":
		return "", &ValidationError{Field: "slug", Message: "must not be empty"}
	case len(slug) < minSlugLength:
		return "", &ValidationError{
			Field:   "slug",
			Message: fmt.Sprintf("must be at least %d characters", minSlugLength),
		}
	case len(slug) > maxSlugLength:
		return "", &ValidationError{
			Field:   "slug",
			Message: fmt.Sprintf("must be at most %d characters", maxSlugLength),
		}
	case !slugPattern.MatchString(slug):
		return "", &ValidationError{
			Field:   "slug",
			Message: "must contain only lowercase letters, digits and single inner hyphens",
		}
	}

	return slug, nil
}

// Valid reports whether the status is a known lifecycle state.
func (s Status) Valid() bool {
	switch s {
	case StatusTrial, StatusActive, StatusSuspended:
		return true
	default:
		return false
	}
}

// Activate moves a trial tenant to active. It uses a value receiver on purpose:
// the returned Tenant is the new state and the receiver is left untouched, so a
// caller cannot mutate shared state by accident.
func (t Tenant) Activate() (Tenant, error) {
	if t.Status != StatusTrial {
		return Tenant{}, fmt.Errorf("%w: cannot activate a %q tenant", ErrInvalidTransition, t.Status)
	}
	t.Status = StatusActive
	return t, nil
}

// Suspend blocks a tenant without destroying data.
func (t Tenant) Suspend() (Tenant, error) {
	if t.Status == StatusSuspended {
		return Tenant{}, fmt.Errorf("%w: tenant is already suspended", ErrInvalidTransition)
	}
	t.Status = StatusSuspended
	return t, nil
}

// Package landing holds the public face of a practice: the text its landing shows at the
// practice's own address (S24, E8/T64). It is pure domain code, like the other aggregates: it
// imports the standard library and the tenant identity, and nothing about HTTP or SQL.
package landing

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// Field limits, in characters. The domain enforces them and migration 0025 mirrors them in
// CHECK constraints, so a row written by any path stays inside them.
const (
	maxDisplayNameLength = 120
	maxTaglineLength     = 160
	maxAboutLength       = 2000
	maxAddressLength     = 200
	maxPhoneLength       = 40
	maxEmailLength       = 254
	maxScheduleLength    = 300
)

// Sentinel errors the delivery layer maps onto transport codes.
var (
	// ErrNotFound reports that the practice has not written its landing yet. Readers answer
	// with the defaults; it is not a failure for the public.
	ErrNotFound = errors.New("landing: profile not found")
	// ErrNotPublished reports that no published practice answers at the address. An unknown
	// host, a host of another domain and a suspended practice all answer this one error, so
	// the public cannot tell which practices exist.
	ErrNotPublished = errors.New("landing: no practice is published at this address")
	// ErrTenantScopeMissing reports a repository call made outside WithTenant, refused
	// before any statement runs.
	ErrTenantScopeMissing = errors.New("landing: no tenant context: the operation must run inside WithTenant")
)

// ValidationError reports a field the domain refuses and names it, so the API can say what to fix.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("landing: invalid %s: %s", e.Field, e.Message)
}

// phonePattern admits digits and the punctuation people write phone numbers with. Letters
// are refused: a phone field that reads "llamar a la secretaria" is a note, not a number.
var phonePattern = regexp.MustCompile(`^\+?[0-9 ()./-]*$`)

// Draft is the text the owner writes. Only the display name is required: a practice can
// publish its name before it has anything else to say.
type Draft struct {
	DisplayName string
	Tagline     string
	About       string
	Address     string
	Phone       string
	Email       string
	Schedule    string
}

// Profile is the public content of one practice.
type Profile struct {
	TenantID tenant.ID
	Draft
	UpdatedAt time.Time
}

// Default is the landing a practice shows before it writes one: its own name and nothing else.
// Nothing is stored for it; the first save creates the row.
func Default(tenantID tenant.ID, practiceName string) Profile {
	return Profile{
		TenantID: tenantID,
		Draft:    Draft{DisplayName: strings.TrimSpace(practiceName)},
	}
}

// New builds the landing of a practice from its first draft.
func New(tenantID tenant.ID, draft Draft, now time.Time) (Profile, error) {
	if tenantID == "" {
		return Profile{}, &ValidationError{Field: "tenantId", Message: "must not be empty"}
	}
	normalized, err := normalize(draft)
	if err != nil {
		return Profile{}, err
	}
	return Profile{TenantID: tenantID, Draft: normalized, UpdatedAt: now.UTC()}, nil
}

// Change replaces the text with a new draft. The practice it belongs to does not change.
func (p Profile) Change(draft Draft, now time.Time) (Profile, error) {
	normalized, err := normalize(draft)
	if err != nil {
		return Profile{}, err
	}
	p.Draft = normalized
	p.UpdatedAt = now.UTC()
	return p, nil
}

// Reconstitute rebuilds a landing from stored state. Adapters must use it instead of a struct
// literal: stored text goes through the same rules as new text.
func Reconstitute(tenantID tenant.ID, draft Draft, updatedAt time.Time) (Profile, error) {
	if tenantID == "" {
		return Profile{}, &ValidationError{Field: "tenantId", Message: "must not be empty"}
	}
	normalized, err := normalize(draft)
	if err != nil {
		return Profile{}, err
	}
	return Profile{TenantID: tenantID, Draft: normalized, UpdatedAt: updatedAt.UTC()}, nil
}

// normalize trims every field and checks the limits. Trimming first means "   " is an empty
// name, not a name of spaces.
func normalize(draft Draft) (Draft, error) {
	d := Draft{
		DisplayName: strings.TrimSpace(draft.DisplayName),
		Tagline:     strings.TrimSpace(draft.Tagline),
		About:       strings.TrimSpace(draft.About),
		Address:     strings.TrimSpace(draft.Address),
		Phone:       strings.TrimSpace(draft.Phone),
		Email:       strings.TrimSpace(draft.Email),
		Schedule:    strings.TrimSpace(draft.Schedule),
	}

	switch {
	case d.DisplayName == "":
		return Draft{}, &ValidationError{Field: "displayName", Message: "must not be empty"}
	case utf8.RuneCountInString(d.DisplayName) > maxDisplayNameLength:
		return Draft{}, lengthError("displayName", maxDisplayNameLength)
	case utf8.RuneCountInString(d.Tagline) > maxTaglineLength:
		return Draft{}, lengthError("tagline", maxTaglineLength)
	case utf8.RuneCountInString(d.About) > maxAboutLength:
		return Draft{}, lengthError("about", maxAboutLength)
	case utf8.RuneCountInString(d.Address) > maxAddressLength:
		return Draft{}, lengthError("address", maxAddressLength)
	case utf8.RuneCountInString(d.Phone) > maxPhoneLength:
		return Draft{}, lengthError("phone", maxPhoneLength)
	case utf8.RuneCountInString(d.Schedule) > maxScheduleLength:
		return Draft{}, lengthError("schedule", maxScheduleLength)
	case !phonePattern.MatchString(d.Phone):
		return Draft{}, &ValidationError{Field: "phone", Message: "may only contain digits, spaces and ( ) . / - and a leading +"}
	}

	if d.Email != "" {
		if utf8.RuneCountInString(d.Email) > maxEmailLength {
			return Draft{}, lengthError("email", maxEmailLength)
		}
		// ParseAddress accepts "Nombre <x@y>" too. The landing publishes a bare address, so the
		// parsed form must be the whole input.
		parsed, err := mail.ParseAddress(d.Email)
		if err != nil || parsed.Address != d.Email {
			return Draft{}, &ValidationError{Field: "email", Message: "must be an email address"}
		}
	}

	return d, nil
}

func lengthError(field string, limit int) error {
	return &ValidationError{Field: field, Message: fmt.Sprintf("must be at most %d characters", limit)}
}

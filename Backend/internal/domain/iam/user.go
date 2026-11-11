package iam

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// UserID is the opaque identity of a user.
type UserID string

// PasswordHash is an opaque, validated password hash. The domain does not hash:
// producing the Argon2id encoded string is the auth layer's job (T14). Here it
// only enforces the database invariant users_password_hash_not_blank, so an
// empty or blank hash can never reach persistence.
type PasswordHash string

// NewPasswordHash validates an already-hashed value. Passing a plaintext here is
// a caller bug, but the domain cannot detect it — and must not try.
func NewPasswordHash(raw string) (PasswordHash, error) {
	if strings.Trim(raw, " ") == "" {
		return "", &ValidationError{Field: "password_hash", Message: "must not be blank"}
	}
	return PasswordHash(raw), nil
}

// emailPattern mirrors the database constraint users_email_shape exactly:
// `^[^@[:space:]]+@[^@[:space:]]+$`. The character class spells out every
// POSIX space instead of \s, because Go's \s does not include vertical tab.
var emailPattern = regexp.MustCompile(`^[^@\t\n\v\f\r ]+@[^@\t\n\v\f\r ]+$`)

// NormalizeEmail mirrors users_email_canonical, `lower(btrim(email))`, exactly:
// btrim strips spaces (not tabs), lower lowercases. The database rejects anything
// else, so a different normalization here would make the per-tenant uniqueness
// fail silently — two spellings of the same address could both be stored.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.Trim(raw, " "))

	switch {
	case email == "":
		return "", &ValidationError{Field: "email", Message: "must not be empty"}
	case !emailPattern.MatchString(email):
		return "", &ValidationError{Field: "email", Message: "must be a plain address like name@example.com"}
	}

	return email, nil
}

// User is a person who works in one tenant. The same human may exist in two
// tenants (the email is unique per tenant, not globally), and each membership
// carries exactly one role.
type User struct {
	ID              UserID
	TenantID        tenant.ID
	Email           string
	PasswordHash    PasswordHash
	FullName        string
	Role            RoleCode
	IsActive        bool
	EmailVerifiedAt *time.Time
	MFASecret       *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// New builds a brand-new user. It is the only constructor that accepts
// unvalidated input: every creation invariant lives here.
func NewUser(id UserID, tenantID tenant.ID, rawEmail, rawHash, fullName string, role RoleCode, now time.Time) (User, error) {
	if id == "" {
		return User{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return User{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}

	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return User{}, err
	}

	hash, err := NewPasswordHash(rawHash)
	if err != nil {
		return User{}, err
	}

	trimmedName := strings.Trim(fullName, " ")
	if trimmedName == "" {
		return User{}, &ValidationError{Field: "full_name", Message: "must not be empty"}
	}

	if !role.Valid() {
		return User{}, &ValidationError{Field: "role", Message: fmt.Sprintf("unknown role %q", role)}
	}

	timestamp := now.UTC()
	return User{
		ID:           id,
		TenantID:     tenantID,
		Email:        email,
		PasswordHash: hash,
		FullName:     trimmedName,
		Role:         role,
		IsActive:     true,
		CreatedAt:    timestamp,
		UpdatedAt:    timestamp,
	}, nil
}

// Reconstitute rebuilds a user from persisted state. Persistence adapters must
// use this instead of a struct literal: it keeps a single gate for the invariants
// that must hold for stored data too — a canonical email, a known role, a
// non-blank hash.
func ReconstituteUser(
	id UserID,
	tenantID tenant.ID,
	email string,
	hash PasswordHash,
	fullName string,
	role RoleCode,
	isActive bool,
	emailVerifiedAt *time.Time,
	mfaSecret *string,
	createdAt, updatedAt time.Time,
) (User, error) {
	if id == "" {
		return User{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return User{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}

	if canonical, err := NormalizeEmail(email); err != nil || canonical != email {
		return User{}, &ValidationError{Field: "email", Message: "stored email is not canonical"}
	}

	if strings.Trim(string(hash), " ") == "" {
		return User{}, &ValidationError{Field: "password_hash", Message: "stored password hash is blank"}
	}

	if strings.Trim(fullName, " ") == "" {
		return User{}, &ValidationError{Field: "full_name", Message: "stored full name is blank"}
	}

	if !role.Valid() {
		return User{}, &ValidationError{Field: "role", Message: fmt.Sprintf("stored role %q is unknown", role)}
	}

	return User{
		ID:              id,
		TenantID:        tenantID,
		Email:           email,
		PasswordHash:    hash,
		FullName:        fullName,
		Role:            role,
		IsActive:        isActive,
		EmailVerifiedAt: emailVerifiedAt,
		MFASecret:       mfaSecret,
		CreatedAt:       createdAt.UTC(),
		UpdatedAt:       updatedAt.UTC(),
	}, nil
}

// Deactivate blocks access without destroying the account. Value receiver on
// purpose, mirroring tenant.Status transitions: the returned User is the new
// state and the receiver is left untouched.
func (u User) Deactivate(now time.Time) (User, error) {
	if !u.IsActive {
		return User{}, fmt.Errorf("%w: user is already inactive", ErrInvalidTransition)
	}
	u.IsActive = false
	u.UpdatedAt = now.UTC()
	return u, nil
}

// Activate restores a deactivated account.
func (u User) Activate(now time.Time) (User, error) {
	if u.IsActive {
		return User{}, fmt.Errorf("%w: user is already active", ErrInvalidTransition)
	}
	u.IsActive = true
	u.UpdatedAt = now.UTC()
	return u, nil
}

// ChangeRole moves the user to another seeded role.
func (u User) ChangeRole(role RoleCode, now time.Time) (User, error) {
	if !role.Valid() {
		return User{}, fmt.Errorf("%w: unknown role %q", ErrInvalidTransition, role)
	}
	u.Role = role
	u.UpdatedAt = now.UTC()
	return u, nil
}

// VerifyEmail marks the address as verified. It is one-way: a re-verification
// would silently move the timestamp and hide a bug, so it is refused.
func (u User) VerifyEmail(now time.Time) (User, error) {
	if u.EmailVerifiedAt != nil {
		return User{}, fmt.Errorf("%w: email is already verified", ErrInvalidTransition)
	}
	verified := now.UTC()
	u.EmailVerifiedAt = &verified
	u.UpdatedAt = verified
	return u, nil
}

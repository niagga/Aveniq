package iam

import (
	"fmt"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// InvitationTTL is how long an invitation link works. Seven days is long enough
// for the invitee to find the message and short enough that a forgotten link does
// not stay a way in.
const InvitationTTL = 7 * 24 * time.Hour

// InvitationID is the opaque identity of an invitation.
type InvitationID string

// Invitation is a pending offer of an account in one practice, with one role.
//
// It is not an account: the account is created when the invitee accepts and
// chooses a password. Until then the invitation only carries the email, the role
// and the hash of the link secret.
type Invitation struct {
	ID         InvitationID
	TenantID   tenant.ID
	Email      string
	Role       RoleCode
	TokenHash  string
	InvitedBy  UserID
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	CreatedAt  time.Time
}

// NewInvitation builds a pending invitation. The email is canonicalised with the
// same rule as accounts, so an invitation for "Ana@X.com" and an account for
// "ana@x.com" collide on the per-tenant uniqueness instead of coexisting.
func NewInvitation(id InvitationID, tenantID tenant.ID, rawEmail string, role RoleCode, tokenHash string, invitedBy UserID, now time.Time) (Invitation, error) {
	if id == "" {
		return Invitation{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Invitation{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if invitedBy == "" {
		return Invitation{}, &ValidationError{Field: "invited_by", Message: "must not be empty"}
	}

	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return Invitation{}, err
	}
	if !role.Valid() {
		return Invitation{}, &ValidationError{Field: "role", Message: fmt.Sprintf("unknown role %q", role)}
	}
	if strings.TrimSpace(tokenHash) == "" {
		return Invitation{}, &ValidationError{Field: "token_hash", Message: "must not be blank"}
	}

	created := now.UTC()
	return Invitation{
		ID:        id,
		TenantID:  tenantID,
		Email:     email,
		Role:      role,
		TokenHash: tokenHash,
		InvitedBy: invitedBy,
		ExpiresAt: created.Add(InvitationTTL),
		CreatedAt: created,
	}, nil
}

// ReconstituteInvitation rebuilds a stored invitation. The inviter is allowed to be
// absent: the foreign key clears it when the inviting account is deleted, and the
// pending offer stays valid without it. Every other invariant is checked again so a
// row that violates one fails loudly on read.
func ReconstituteInvitation(
	id InvitationID,
	tenantID tenant.ID,
	email string,
	role RoleCode,
	tokenHash string,
	invitedBy UserID,
	expiresAt time.Time,
	acceptedAt *time.Time,
	createdAt time.Time,
) (Invitation, error) {
	if id == "" {
		return Invitation{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Invitation{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	canonical, err := NormalizeEmail(email)
	if err != nil {
		return Invitation{}, err
	}
	if canonical != email {
		return Invitation{}, &ValidationError{Field: "email", Message: "stored email is not canonical"}
	}
	if !role.Valid() {
		return Invitation{}, &ValidationError{Field: "role", Message: fmt.Sprintf("unknown role %q", role)}
	}
	if strings.TrimSpace(tokenHash) == "" {
		return Invitation{}, &ValidationError{Field: "token_hash", Message: "must not be blank"}
	}

	invitation := Invitation{
		ID:        id,
		TenantID:  tenantID,
		Email:     email,
		Role:      role,
		TokenHash: tokenHash,
		InvitedBy: invitedBy,
		ExpiresAt: expiresAt.UTC(),
		CreatedAt: createdAt.UTC(),
	}
	if acceptedAt != nil {
		accepted := acceptedAt.UTC()
		invitation.AcceptedAt = &accepted
	}
	return invitation, nil
}

// Usable reports whether the invitation can still be accepted at now. Accepted
// and expired invitations answer ErrInvitationUnusable, and the transport maps
// both to the same answer: the link no longer opens an account.
func (i Invitation) Usable(now time.Time) error {
	if i.AcceptedAt != nil {
		return fmt.Errorf("%w: already accepted", ErrInvitationUnusable)
	}
	if !now.Before(i.ExpiresAt) {
		return fmt.Errorf("%w: expired", ErrInvitationUnusable)
	}
	return nil
}

// Accept marks the invitation as used at now. It is a value transition like the
// rest of the aggregate; the single-use guarantee across concurrent requests is
// the storage's job (a conditional UPDATE), not this method's.
func (i Invitation) Accept(now time.Time) (Invitation, error) {
	if err := i.Usable(now); err != nil {
		return Invitation{}, err
	}
	accepted := now.UTC()
	i.AcceptedAt = &accepted
	return i, nil
}

// AuthorizeInvitation decides whether actor may invite someone to role. Same rule
// as AuthorizeChange for the owner role: only an owner hands out the owner role,
// so an admin can invite staff but cannot create a second owner.
//
// The same check applies to revoking: a pending owner invitation is an owner
// decision, whoever wrote it.
func AuthorizeInvitation(actor Actor, role RoleCode) error {
	if actor.Role != RoleOwner && role == RoleOwner {
		return fmt.Errorf("%w: only an owner invites someone as owner", ErrChangeNotAllowed)
	}
	return nil
}

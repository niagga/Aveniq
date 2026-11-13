// Package invitationapp is the invitation flow: an owner or admin with
// users:manage offers an account in the practice, the invitee opens the link,
// chooses a password and the account is created. The link is the only channel
// (no email in this phase), so the token is shown to the inviter once and the
// database keeps only its hash.
package invitationapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// Pagination bounds, same policy as the user directory.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// Caller is who asks: the identity the verified access token carries.
type Caller struct {
	TenantID string
	UserID   string
	Role     string
}

// PasswordHasher produces the stored hash for the password the invitee chooses.
// The application depends on this narrow contract, like auth.PasswordVerifier, so
// tests do not pay the real argon2id cost.
type PasswordHasher interface {
	Hash(password string) (string, error)
}

// InvitationView is a pending invitation as the administrative screens read it.
// It never carries the token or its hash.
type InvitationView struct {
	ID        string
	Email     string
	Role      string
	InvitedBy string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Issued is the answer to a creation. Token is the only time the secret leaves the
// server; the screen shows it to the inviter to copy into the link.
type Issued struct {
	Invitation InvitationView
	Token      string
}

// Preview is what the public link page may show before the invitee acts: enough to
// say who is inviting them to which role, and nothing about the practice's data.
type Preview struct {
	Email     string
	Role      string
	ExpiresAt time.Time
}

// Accepted is the account created by an acceptance.
type Accepted struct {
	UserID   string
	TenantID string
	Email    string
	Role     string
}

func toView(invitation iam.Invitation) InvitationView {
	return InvitationView{
		ID:        string(invitation.ID),
		Email:     invitation.Email,
		Role:      string(invitation.Role),
		InvitedBy: string(invitation.InvitedBy),
		ExpiresAt: invitation.ExpiresAt,
		CreatedAt: invitation.CreatedAt,
	}
}

// deps is what the flows share. The grouping keeps a missing collaborator from
// slipping in one constructor at a time.
type deps struct {
	invitations port.InvitationRepository
	users       port.UserRepository
	scope       port.TenantScope
	audit       port.AuditLog
	clock       port.Clock
	ids         port.IDGenerator
}

func actorOf(who Caller) iam.Actor {
	return iam.Actor{ID: iam.UserID(who.UserID), Role: iam.RoleCode(who.Role)}
}

// ErrLinkUnusable is what every failure on the public link answers, a malformed
// token included. The invitee learns that the link does not open an account, and
// nothing about why or whether the token exists elsewhere.
var ErrLinkUnusable = iam.ErrInvitationUnusable

// ─── issuing ─────────────────────────────────────────────────────────────────

// CreateInput offers an account in the caller's practice.
type CreateInput struct {
	Caller
	Email string
	Role  string
}

// CreateUseCase issues an invitation.
type CreateUseCase struct {
	deps
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(invitations port.InvitationRepository, users port.UserRepository, scope port.TenantScope, audit port.AuditLog, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{deps{invitations: invitations, users: users, scope: scope, audit: audit, clock: clock, ids: ids}}
}

// Execute issues the invitation and returns its token once. An email that already
// has an account in the practice is refused here, not at acceptance, so the
// inviter learns it while the invitation can still be withdrawn.
func (uc *CreateUseCase) Execute(ctx context.Context, in CreateInput) (Issued, error) {
	email, err := iam.NormalizeEmail(in.Email)
	if err != nil {
		return Issued{}, err
	}
	role := iam.RoleCode(in.Role)
	if !role.Valid() {
		return Issued{}, &iam.ValidationError{Field: "role", Message: "must be one of the seeded roles"}
	}
	if err := iam.AuthorizeInvitation(actorOf(in.Caller), role); err != nil {
		return Issued{}, err
	}

	token, err := authn.NewInvitationToken(in.TenantID)
	if err != nil {
		return Issued{}, err
	}

	var issued Issued
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		_, err := uc.users.ByEmail(ctx, email)
		switch {
		case err == nil:
			return iam.ErrEmailTaken
		case !errors.Is(err, iam.ErrNotFound):
			return err
		}

		now := uc.clock.Now()
		invitation, err := iam.NewInvitation(
			iam.InvitationID(uc.ids.NewID()),
			tenant.ID(in.TenantID),
			email,
			role,
			token.Hash,
			iam.UserID(in.UserID),
			now,
		)
		if err != nil {
			return err
		}
		if err := uc.invitations.Create(ctx, invitation); err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.UserID,
			Action:   "invitation.created",
			Entity:   "invitation",
			EntityID: string(invitation.ID),
			Metadata: map[string]any{"role": string(role)},
		}); err != nil {
			return err
		}

		issued = Issued{Invitation: toView(invitation), Token: token.Raw}
		return nil
	})
	if err != nil {
		return Issued{}, err
	}
	return issued, nil
}

// ListInput requests one page of pending invitations.
type ListInput struct {
	Caller
	Limit  int32
	Offset int32
}

// ListPendingUseCase lists the invitations of the practice that can still be used.
type ListPendingUseCase struct {
	deps
}

// NewListPendingUseCase wires the use case.
func NewListPendingUseCase(invitations port.InvitationRepository, scope port.TenantScope, clock port.Clock) *ListPendingUseCase {
	return &ListPendingUseCase{deps{invitations: invitations, scope: scope, clock: clock}}
}

// Execute returns one page, newest first. Accepted and expired invitations are not
// pending and do not appear.
func (uc *ListPendingUseCase) Execute(ctx context.Context, in ListInput) ([]InvitationView, error) {
	limit := in.Limit
	if limit == 0 {
		limit = DefaultPageSize
	}
	if limit < 0 || limit > MaxPageSize {
		return nil, &iam.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	if in.Offset < 0 {
		return nil, &iam.ValidationError{Field: "offset", Message: "must not be negative"}
	}

	var views []InvitationView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		rows, err := uc.invitations.Pending(ctx, uc.clock.Now(), limit, in.Offset)
		if err != nil {
			return err
		}
		views = make([]InvitationView, 0, len(rows))
		for _, row := range rows {
			views = append(views, toView(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// RevokeUseCase withdraws a pending invitation.
type RevokeUseCase struct {
	deps
}

// NewRevokeUseCase wires the use case.
func NewRevokeUseCase(invitations port.InvitationRepository, scope port.TenantScope, audit port.AuditLog) *RevokeUseCase {
	return &RevokeUseCase{deps{invitations: invitations, scope: scope, audit: audit}}
}

// Execute revokes the invitation. An accepted one is history and cannot be revoked
// (invalid transition); a foreign or unknown one is not found. The owner-only rule
// applies to the invitation's role, so an admin cannot withdraw an owner offer.
func (uc *RevokeUseCase) Execute(ctx context.Context, who Caller, invitationID string) error {
	return uc.scope.WithTenant(ctx, who.TenantID, func(ctx context.Context) error {
		invitation, err := uc.invitations.ByID(ctx, iam.InvitationID(invitationID))
		if err != nil {
			return err
		}
		if err := iam.AuthorizeInvitation(actorOf(who), invitation.Role); err != nil {
			return err
		}
		if invitation.AcceptedAt != nil {
			return fmt.Errorf("%w: the invitation was already accepted", iam.ErrInvalidTransition)
		}
		if err := uc.invitations.Delete(ctx, invitation.ID); err != nil {
			return err
		}
		return uc.audit.Record(ctx, port.AuditEvent{
			TenantID: who.TenantID,
			ActorID:  who.UserID,
			Action:   "invitation.revoked",
			Entity:   "invitation",
			EntityID: invitationID,
			Metadata: map[string]any{"role": string(invitation.Role)},
		})
	})
}

// ─── public link ─────────────────────────────────────────────────────────────

// PreviewUseCase reads an invitation from its link, without authentication.
type PreviewUseCase struct {
	deps
}

// NewPreviewUseCase wires the use case.
func NewPreviewUseCase(invitations port.InvitationRepository, scope port.TenantScope, clock port.Clock) *PreviewUseCase {
	return &PreviewUseCase{deps{invitations: invitations, scope: scope, clock: clock}}
}

// Execute resolves the link. Every failure answers ErrLinkUnusable.
func (uc *PreviewUseCase) Execute(ctx context.Context, rawToken string) (Preview, error) {
	token, err := authn.ParseInvitationToken(rawToken)
	if err != nil {
		return Preview{}, ErrLinkUnusable
	}

	var preview Preview
	err = uc.scope.WithTenant(ctx, token.TenantID, func(ctx context.Context) error {
		invitation, err := lookupByHash(ctx, uc.invitations, token.Hash)
		if err != nil {
			return err
		}
		if err := invitation.Usable(uc.clock.Now()); err != nil {
			return err
		}
		preview = Preview{Email: invitation.Email, Role: string(invitation.Role), ExpiresAt: invitation.ExpiresAt}
		return nil
	})
	if err != nil {
		return Preview{}, err
	}
	return preview, nil
}

// AcceptInput is what the invitee sends from the link page.
type AcceptInput struct {
	Token    string
	FullName string
	Password string
}

// AcceptUseCase turns an invitation into an account.
type AcceptUseCase struct {
	deps
	hasher PasswordHasher
}

// NewAcceptUseCase wires the use case.
func NewAcceptUseCase(
	invitations port.InvitationRepository,
	users port.UserRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	ids port.IDGenerator,
	hasher PasswordHasher,
) *AcceptUseCase {
	return &AcceptUseCase{
		deps:   deps{invitations: invitations, users: users, scope: scope, audit: audit, clock: clock, ids: ids},
		hasher: hasher,
	}
}

// Execute creates the account and marks the invitation used, in one tenant
// transaction. Shape and password checks run first and cost nothing; the hash is
// computed before the transaction opens so argon2 does not hold a connection. If
// the invitation was used or expired in the meantime, the account insert rolls back.
func (uc *AcceptUseCase) Execute(ctx context.Context, in AcceptInput) (Accepted, error) {
	token, err := authn.ParseInvitationToken(in.Token)
	if err != nil {
		return Accepted{}, ErrLinkUnusable
	}
	if err := iam.ValidatePassword(in.Password); err != nil {
		return Accepted{}, err
	}
	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return Accepted{}, fmt.Errorf("invitationapp: hash password: %w", err)
	}

	var accepted Accepted
	err = uc.scope.WithTenant(ctx, token.TenantID, func(ctx context.Context) error {
		invitation, err := lookupByHash(ctx, uc.invitations, token.Hash)
		if err != nil {
			return err
		}
		now := uc.clock.Now()
		if err := invitation.Usable(now); err != nil {
			return err
		}

		userID := uc.ids.NewID()
		user, err := iam.NewUser(iam.UserID(userID), invitation.TenantID, invitation.Email, hash, in.FullName, invitation.Role, now)
		if err != nil {
			return err
		}
		if err := uc.users.Create(ctx, user); err != nil {
			return err
		}
		if err := uc.invitations.MarkAccepted(ctx, invitation.ID, now); err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: string(invitation.TenantID),
			ActorID:  userID,
			Action:   "invitation.accepted",
			Entity:   "invitation",
			EntityID: string(invitation.ID),
			Metadata: map[string]any{"role": string(invitation.Role), "user_id": userID},
		}); err != nil {
			return err
		}

		accepted = Accepted{
			UserID:   userID,
			TenantID: string(invitation.TenantID),
			Email:    invitation.Email,
			Role:     string(invitation.Role),
		}
		return nil
	})
	if err != nil {
		return Accepted{}, err
	}
	return accepted, nil
}

// lookupByHash resolves a link and folds "no such link" into ErrLinkUnusable.
func lookupByHash(ctx context.Context, invitations port.InvitationRepository, hash string) (iam.Invitation, error) {
	invitation, err := invitations.ByTokenHash(ctx, hash)
	if errors.Is(err, iam.ErrNotFound) {
		return iam.Invitation{}, ErrLinkUnusable
	}
	return invitation, err
}

package auth

import (
	"context"
	"errors"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// RefreshInput carries the token the client presents.
type RefreshInput struct {
	RefreshToken string
}

// RefreshDeps are the collaborators of the refresh use case.
type RefreshDeps struct {
	Users    port.UserRepository
	Sessions port.SessionRepository
	Scope    port.TenantScope
	Tokens   AccessTokenIssuer
	Clock    port.Clock
	IDs      port.IDGenerator
}

// RefreshUseCase rotates a grant: the presented token is spent and a new pair is
// issued. Presenting a spent token again is the reuse signal.
type RefreshUseCase struct {
	deps   RefreshDeps
	issuer sessionIssuer
}

// NewRefreshUseCase wires the use case.
func NewRefreshUseCase(deps RefreshDeps) *RefreshUseCase {
	return &RefreshUseCase{
		deps: deps,
		issuer: sessionIssuer{
			sessions: deps.Sessions,
			tokens:   deps.Tokens,
			clock:    deps.Clock,
			ids:      deps.IDs,
		},
	}
}

// Execute rotates the session. The token carries the tenant as a lookup hint, so
// the scope can be opened before asking row-level security anything: the tenant
// segment is never a permission, and the hash covers the whole string, so a moved
// secret matches no row.
func (uc *RefreshUseCase) Execute(ctx context.Context, in RefreshInput) (Session, error) {
	parsed, err := authn.ParseRefreshToken(in.RefreshToken)
	if err != nil {
		return Session{}, ErrInvalidRefreshToken
	}

	var issued Session
	err = uc.deps.Scope.WithTenant(ctx, parsed.TenantID, func(ctx context.Context) error {
		current, err := uc.deps.Sessions.ByRefreshHash(ctx, iam.RefreshHash(parsed.Hash))
		if err != nil {
			if errors.Is(err, iam.ErrNotFound) {
				return ErrInvalidRefreshToken
			}
			return err
		}

		now := uc.deps.Clock.Now()
		if current.RevokedAt != nil {
			// Decision 3 of L11: the stolen copy and the legitimate client both hold
			// a token of this chain and nothing here can tell them apart, so the whole
			// line dies and a fresh login is required.
			return uc.revokeChain(ctx, current.ID, now, ErrRefreshReused)
		}
		if !now.Before(current.ExpiresAt) {
			return ErrSessionExpired
		}

		user, err := uc.deps.Users.ByID(ctx, current.UserID)
		if err != nil {
			if errors.Is(err, iam.ErrNotFound) {
				return uc.revokeChain(ctx, current.ID, now, ErrInvalidRefreshToken)
			}
			return err
		}
		if !user.IsActive {
			return uc.revokeChain(ctx, current.ID, now, ErrInvalidRefreshToken)
		}

		successor, refreshToken, err := uc.issuer.newSession(user)
		if err != nil {
			return err
		}

		// The domain owns the transition; the adapter persists both halves in one
		// transaction, so a failure leaves neither.
		rotated, err := current.ReplaceWith(successor.ID, now)
		if err != nil {
			return err
		}
		if err := uc.deps.Sessions.Rotate(ctx, rotated, successor); err != nil {
			return err
		}

		issued, err = uc.issuer.pair(user, refreshToken, successor.ExpiresAt)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return issued, nil
}

// revokeChain ends every session reachable from the presented one and returns the
// cause, so the caller cannot confuse a replay with an expired grant. A revoke that
// finds nothing is not an error: the point was that nothing stays alive.
func (uc *RefreshUseCase) revokeChain(ctx context.Context, id iam.SessionID, now time.Time, cause error) error {
	if _, err := uc.deps.Sessions.RevokeChain(ctx, id, now); err != nil && !errors.Is(err, iam.ErrNotFound) {
		return err
	}
	return cause
}

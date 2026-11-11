package auth

import (
	"context"
	"errors"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// LogoutInput carries the token the client wants to end.
type LogoutInput struct {
	RefreshToken string
}

// LogoutDeps are the collaborators of the logout use case.
type LogoutDeps struct {
	Sessions port.SessionRepository
	Scope    port.TenantScope
	Clock    port.Clock
}

// LogoutUseCase revokes the presented session.
type LogoutUseCase struct {
	deps LogoutDeps
}

// NewLogoutUseCase wires the use case.
func NewLogoutUseCase(deps LogoutDeps) *LogoutUseCase {
	return &LogoutUseCase{deps: deps}
}

// Execute revokes the grant and always succeeds.
//
// Logout must not be an oracle: an unparseable token, a token that matches no
// session and a token already revoked all answer the same thing, because the only
// thing the caller can act on is "you are logged out". Reporting a difference
// would let anyone test whether a guessed token exists.
func (uc *LogoutUseCase) Execute(ctx context.Context, in LogoutInput) error {
	parsed, err := authn.ParseRefreshToken(in.RefreshToken)
	if err != nil {
		return nil
	}

	return uc.deps.Scope.WithTenant(ctx, parsed.TenantID, func(ctx context.Context) error {
		current, err := uc.deps.Sessions.ByRefreshHash(ctx, iam.RefreshHash(parsed.Hash))
		if err != nil {
			if errors.Is(err, iam.ErrNotFound) {
				return nil
			}
			return err
		}
		if current.RevokedAt != nil {
			return nil
		}

		if err := uc.deps.Sessions.Revoke(ctx, current.ID, uc.deps.Clock.Now()); err != nil && !errors.Is(err, iam.ErrNotFound) {
			return err
		}
		return nil
	})
}

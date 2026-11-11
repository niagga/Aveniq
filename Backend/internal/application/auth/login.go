package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// LoginInput is what the client chooses. The tenant is part of it because the
// email is unique per tenant, not platform-wide: the same person may exist in two
// practices (decision 1 of L11).
type LoginInput struct {
	TenantSlug string
	Email      string
	Password   string
}

// LoginDeps are the collaborators of the login use case.
type LoginDeps struct {
	Tenants  port.TenantRepository
	Users    port.UserRepository
	Sessions port.SessionRepository
	Scope    port.TenantScope
	Verifier PasswordVerifier
	Tokens   AccessTokenIssuer
	Clock    port.Clock
	IDs      port.IDGenerator
	// DummyHash is verified when there is no real hash to verify, so that an
	// unknown email costs the same wall time as a known one.
	DummyHash string
}

// LoginUseCase authenticates a password and opens a refresh session.
type LoginUseCase struct {
	deps   LoginDeps
	issuer sessionIssuer
}

// NewLoginUseCase wires the use case. A missing dummy hash is a configuration
// error rather than a warning: without it, response time tells an attacker which
// addresses exist in a health SaaS.
func NewLoginUseCase(deps LoginDeps) (*LoginUseCase, error) {
	if deps.DummyHash == "" {
		return nil, errors.New("auth: login needs DummyHash to keep an unknown user indistinguishable by time")
	}
	return &LoginUseCase{
		deps: deps,
		issuer: sessionIssuer{
			sessions: deps.Sessions,
			tokens:   deps.Tokens,
			clock:    deps.Clock,
			ids:      deps.IDs,
		},
	}, nil
}

// Execute resolves the tenant by slug outside the tenant scope — the tenants table
// has no row-level security and is the entry point itself — and runs everything
// else inside WithTenant, where the user and session policies apply.
func (uc *LoginUseCase) Execute(ctx context.Context, in LoginInput) (Session, error) {
	subject, err := uc.deps.Tenants.BySlug(ctx, in.TenantSlug)
	if err != nil {
		if errors.Is(err, tenant.ErrNotFound) {
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, err
	}

	email, err := iam.NormalizeEmail(in.Email)
	if err != nil {
		// An address the domain would never store cannot belong to a user, and the
		// answer is the same one a wrong password gets.
		return Session{}, ErrInvalidCredentials
	}

	var issued Session
	err = uc.deps.Scope.WithTenant(ctx, string(subject.ID), func(ctx context.Context) error {
		user, err := uc.deps.Users.ByEmail(ctx, email)
		if err != nil {
			if errors.Is(err, iam.ErrNotFound) {
				uc.spendVerificationTime(in.Password)
				return ErrInvalidCredentials
			}
			return err
		}

		// A deactivated user costs the same as a wrong password, and the hash is
		// still verified: returning early would make "this account exists but was
		// disabled" measurable.
		if !user.IsActive {
			uc.spendVerificationTime(in.Password)
			return ErrInvalidCredentials
		}

		if err := uc.deps.Verifier.Verify(in.Password, string(user.PasswordHash)); err != nil {
			if errors.Is(err, authn.ErrPasswordMismatch) {
				return ErrInvalidCredentials
			}
			// A stored hash this package cannot parse is a corrupt row, not a bad
			// guess: it must surface as an incident instead of a rejected login.
			return fmt.Errorf("auth: stored password hash is unusable: %w", err)
		}

		session, refreshToken, err := uc.issuer.newSession(user)
		if err != nil {
			return err
		}
		if err := uc.deps.Sessions.Create(ctx, session); err != nil {
			return err
		}

		issued, err = uc.issuer.pair(user, refreshToken, session.ExpiresAt)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return issued, nil
}

// spendVerificationTime runs a real verification against the dummy hash. Doing
// nothing would be far cheaper than hashing, and that difference is the oracle.
func (uc *LoginUseCase) spendVerificationTime(password string) {
	_ = uc.deps.Verifier.Verify(password, uc.deps.DummyHash)
}

// Package signupapp is the public registration of a practice: one call creates the
// practice in trial and its first account, the owner. It is the only way a practice
// comes into existence from outside the platform, so it is also the place where the
// trial starts and where the first owner is born.
//
// Email verification is not part of this flow yet: the owner's address stays
// unverified until a mail provider exists (T36 decision). Signing in does not check
// verification, so the practice is usable immediately and the gap is explicit.
package signupapp

import (
	"context"
	"fmt"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// PasswordHasher produces the stored hash for the owner's password. Narrow on
// purpose, like the other application packages, so tests skip the argon2id cost.
type PasswordHasher interface {
	Hash(password string) (string, error)
}

// Input is what the registrant chooses. Identity, status and timestamps are decided
// by the system and are deliberately absent.
type Input struct {
	PracticeName string
	Slug         string
	OwnerName    string
	OwnerEmail   string
	Password     string
}

// Result is the practice and its owner as created. It carries no session: the owner
// signs in with the password they just chose.
type Result struct {
	TenantID    string
	Slug        string
	Name        string
	Status      string
	Timezone    string
	CreatedAt   time.Time
	TrialEndsAt time.Time
	OwnerID     string
	OwnerEmail  string
	OwnerRole   string
}

// UseCase registers a practice and its owner in one tenant transaction.
type UseCase struct {
	tenants port.TenantRepository
	users   port.UserRepository
	scope   port.TenantScope
	audit   port.AuditLog
	clock   port.Clock
	ids     port.IDGenerator
	hasher  PasswordHasher
}

// NewUseCase wires the use case. Every collaborator is explicit so the transaction
// boundary is visible in one place.
func NewUseCase(
	tenants port.TenantRepository,
	users port.UserRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	ids port.IDGenerator,
	hasher PasswordHasher,
) *UseCase {
	return &UseCase{tenants: tenants, users: users, scope: scope, audit: audit, clock: clock, ids: ids, hasher: hasher}
}

// Execute creates the practice and the owner, or returns the first violated rule.
// The password and the practice rules run before the hash, and nothing is written
// until every rule has passed, so a refused request never opens a transaction.
//
// A slug already taken answers tenant.ErrSlugTaken from the insert inside the
// transaction; the owner insert never runs for it, and nothing is left behind.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Result, error) {
	if err := iam.ValidatePassword(in.Password); err != nil {
		return Result{}, err
	}

	now := uc.clock.Now()
	practiceID := tenant.ID(uc.ids.NewID())
	practice, err := tenant.New(practiceID, in.Slug, in.PracticeName, now)
	if err != nil {
		return Result{}, err
	}

	// The owner's row needs its hash at construction, so the hash comes before the
	// email and name checks. An invalid email therefore costs one argon2id run; the
	// rate limit on this route bounds that.
	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return Result{}, fmt.Errorf("signupapp: hash password: %w", err)
	}

	ownerID := iam.UserID(uc.ids.NewID())
	owner, err := iam.NewUser(ownerID, practice.ID, in.OwnerEmail, hash, in.OwnerName, iam.RoleOwner, now)
	if err != nil {
		return Result{}, err
	}

	err = uc.scope.WithTenant(ctx, string(practice.ID), func(ctx context.Context) error {
		if err := uc.tenants.Create(ctx, practice); err != nil {
			return err
		}
		if err := uc.users.Create(ctx, owner); err != nil {
			return err
		}
		return uc.audit.Record(ctx, port.AuditEvent{
			TenantID: string(practice.ID),
			ActorID:  string(ownerID),
			Action:   "tenant.registered",
			Entity:   "tenant",
			EntityID: string(practice.ID),
			Metadata: map[string]any{"owner_role": string(iam.RoleOwner)},
		})
	})
	if err != nil {
		return Result{}, err
	}

	return Result{
		TenantID:    string(practice.ID),
		Slug:        practice.Slug,
		Name:        practice.Name,
		Status:      string(practice.Status),
		Timezone:    practice.Timezone,
		CreatedAt:   practice.CreatedAt,
		TrialEndsAt: practice.TrialEndsAt,
		OwnerID:     string(ownerID),
		OwnerEmail:  owner.Email,
		OwnerRole:   string(owner.Role),
	}, nil
}

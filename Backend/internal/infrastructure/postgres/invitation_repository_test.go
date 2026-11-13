package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// invitationEnv adds the invitation adapter to the IAM scratch database. An
// invitation references its inviter, so the tests also need accounts to hold that
// role; the user repository seeds them.
func invitationEnv(t *testing.T) (*postgres.Transactor, *postgres.UserRepository, *postgres.InvitationRepository, string, string) {
	t.Helper()

	pool, transactor, users, tenantA, tenantB := iamEnv(t)
	return transactor, users, postgres.NewInvitationRepository(pool), tenantA, tenantB
}

// seedInviter creates the account that issues the invitations in tenantID.
func seedInviter(t *testing.T, transactor *postgres.Transactor, users *postgres.UserRepository, tenantID string) iam.UserID {
	t.Helper()

	owner := newUser(t, tenantID, "owner-"+uuid.NewString()[:8]+"@practica.test")
	owner.Role = iam.RoleOwner
	if err := transactor.WithTenant(context.Background(), tenantID, func(ctx context.Context) error {
		return users.Create(ctx, owner)
	}); err != nil {
		t.Fatalf("cannot seed the inviter: %v", err)
	}
	return owner.ID
}

// newInvitation builds a domain invitation issued at now (so the expiry follows).
func newInvitation(t *testing.T, tenantID string, inviter iam.UserID, email string, now time.Time) iam.Invitation {
	t.Helper()

	invitation, err := iam.NewInvitation(
		iam.InvitationID(uuid.NewString()),
		tenant.ID(tenantID),
		email,
		iam.RoleProfessional,
		"hash-"+uuid.NewString(),
		inviter,
		now,
	)
	if err != nil {
		t.Fatalf("cannot build the invitation: %v", err)
	}
	return invitation
}

func TestInvitationRepository(t *testing.T) {
	transactor, users, invitations, tenantA, tenantB := invitationEnv(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	inviter := seedInviter(t, transactor, users, tenantA)
	pending := newInvitation(t, tenantA, inviter, "ana@practica.test", now)

	t.Run("an invitation round-trips through the adapter", func(t *testing.T) {
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return invitations.Create(ctx, pending)
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		var got iam.Invitation
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var err error
			got, err = invitations.ByID(ctx, pending.ID)
			return err
		}); err != nil {
			t.Fatalf("ByID: %v", err)
		}
		if got.Email != pending.Email || got.Role != pending.Role || got.TokenHash != pending.TokenHash {
			t.Fatalf("round trip changed the invitation: got %+v, want %+v", got, pending)
		}
		if got.InvitedBy != inviter {
			t.Fatalf("invited_by = %q, want %q", got.InvitedBy, inviter)
		}
		if !got.ExpiresAt.Equal(pending.ExpiresAt) {
			t.Fatalf("expiry = %v, want %v", got.ExpiresAt, pending.ExpiresAt)
		}
	})

	t.Run("the token hash resolves the invitation", func(t *testing.T) {
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := invitations.ByTokenHash(ctx, pending.TokenHash)
			if err != nil {
				return err
			}
			if got.ID != pending.ID {
				t.Fatalf("ByTokenHash returned %s, want %s", got.ID, pending.ID)
			}
			return nil
		}); err != nil {
			t.Fatalf("ByTokenHash: %v", err)
		}
	})

	t.Run("another practice sees nothing: same hash, same id, the pending list", func(t *testing.T) {
		if err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			if _, err := invitations.ByID(ctx, pending.ID); !errors.Is(err, iam.ErrNotFound) {
				t.Fatalf("ByID across practices = %v, want ErrNotFound", err)
			}
			if _, err := invitations.ByTokenHash(ctx, pending.TokenHash); !errors.Is(err, iam.ErrNotFound) {
				t.Fatalf("ByTokenHash across practices = %v, want ErrNotFound", err)
			}
			rows, err := invitations.Pending(ctx, now, 20, 0)
			if err != nil {
				return err
			}
			if len(rows) != 0 {
				t.Fatalf("Pending across practices = %d rows, want 0", len(rows))
			}
			if err := invitations.Delete(ctx, pending.ID); !errors.Is(err, iam.ErrNotFound) {
				t.Fatalf("Delete across practices = %v, want ErrNotFound", err)
			}
			return nil
		}); err != nil {
			t.Fatalf("cross-practice reads: %v", err)
		}
	})

	t.Run("without a tenant scope the adapter refuses to run", func(t *testing.T) {
		if err := invitations.Create(ctx, newInvitation(t, tenantA, inviter, "x@practica.test", now)); !errors.Is(err, iam.ErrTenantScopeMissing) {
			t.Fatalf("Create outside a scope = %v, want ErrTenantScopeMissing", err)
		}
		if _, err := invitations.ByTokenHash(ctx, pending.TokenHash); !errors.Is(err, iam.ErrTenantScopeMissing) {
			t.Fatalf("ByTokenHash outside a scope = %v, want ErrTenantScopeMissing", err)
		}
	})

	t.Run("an expired invitation cannot be marked accepted", func(t *testing.T) {
		old := newInvitation(t, tenantA, inviter, "vieja@practica.test", now.Add(-30*24*time.Hour))
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return invitations.Create(ctx, old)
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return invitations.MarkAccepted(ctx, old.ID, now)
		}); !errors.Is(err, iam.ErrInvitationUnusable) {
			t.Fatalf("MarkAccepted of an expired invitation = %v, want ErrInvitationUnusable", err)
		}
	})

	t.Run("two racing acceptances of one link: exactly one wins", func(t *testing.T) {
		raced := newInvitation(t, tenantA, inviter, "carrera@practica.test", now)
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return invitations.Create(ctx, raced)
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}

		const contenders = 8
		results := make(chan error, contenders)
		var wg sync.WaitGroup
		for range contenders {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
					return invitations.MarkAccepted(ctx, raced.ID, now)
				})
			}()
		}
		wg.Wait()
		close(results)

		wins, losses := 0, 0
		for err := range results {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, iam.ErrInvitationUnusable):
				losses++
			default:
				t.Fatalf("unexpected error from a contender: %v", err)
			}
		}
		if wins != 1 || losses != contenders-1 {
			t.Fatalf("wins = %d, losses = %d; the single-use guard must admit exactly one", wins, losses)
		}
	})

	t.Run("the pending list leaves out accepted and expired invitations", func(t *testing.T) {
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			rows, err := invitations.Pending(ctx, now, 100, 0)
			if err != nil {
				return err
			}
			for _, row := range rows {
				if row.AcceptedAt != nil {
					t.Fatalf("accepted invitation %s is listed as pending", row.ID)
				}
				if !now.Before(row.ExpiresAt) {
					t.Fatalf("expired invitation %s is listed as pending", row.ID)
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("Pending: %v", err)
		}
	})

	t.Run("an accepted invitation cannot be deleted, and stays readable", func(t *testing.T) {
		accepted := newInvitation(t, tenantA, inviter, "historia@practica.test", now)
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			if err := invitations.Create(ctx, accepted); err != nil {
				return err
			}
			return invitations.MarkAccepted(ctx, accepted.ID, now)
		}); err != nil {
			t.Fatalf("create and accept: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			if err := invitations.Delete(ctx, accepted.ID); !errors.Is(err, iam.ErrNotFound) {
				t.Fatalf("Delete of an accepted invitation = %v, want ErrNotFound", err)
			}
			got, err := invitations.ByID(ctx, accepted.ID)
			if err != nil {
				return err
			}
			if got.AcceptedAt == nil {
				t.Fatal("the accepted invitation lost its acceptance")
			}
			return nil
		}); err != nil {
			t.Fatalf("after accept: %v", err)
		}
	})

	t.Run("a pending invitation can be revoked", func(t *testing.T) {
		revocable := newInvitation(t, tenantA, inviter, "revocada@practica.test", now)
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			if err := invitations.Create(ctx, revocable); err != nil {
				return err
			}
			return invitations.Delete(ctx, revocable.ID)
		}); err != nil {
			t.Fatalf("create and revoke: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, err := invitations.ByID(ctx, revocable.ID)
			return err
		}); !errors.Is(err, iam.ErrNotFound) {
			t.Fatalf("ByID after revoke = %v, want ErrNotFound", err)
		}
	})

	t.Run("an invitation survives the removal of its inviter", func(t *testing.T) {
		departing := seedInviter(t, transactor, users, tenantA)
		orphan := newInvitation(t, tenantA, departing, "huerfana@practica.test", now)
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return invitations.Create(ctx, orphan)
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return users.Delete(ctx, departing)
		}); err != nil {
			t.Fatalf("delete the inviter: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := invitations.ByID(ctx, orphan.ID)
			if err != nil {
				return err
			}
			if got.InvitedBy != "" {
				t.Fatalf("invited_by = %q after the inviter was removed, want empty", got.InvitedBy)
			}
			if got.AcceptedAt != nil || !now.Before(got.ExpiresAt) {
				t.Fatalf("the pending offer must stay usable without its inviter: %+v", got)
			}
			return nil
		}); err != nil {
			t.Fatalf("read orphan: %v", err)
		}
	})
}

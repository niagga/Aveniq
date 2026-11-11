package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// sessionEnv is iamEnv plus the session adapter, against its own throwaway
// database. It skips cleanly without the DSNs, like the rest of the package.
func sessionEnv(t *testing.T) (*postgres.Transactor, *postgres.UserRepository, *postgres.SessionRepository, string, string) {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the session repository tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	pool, err := pgxpool.New(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database as the application role: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return postgres.NewTransactor(pool),
		postgres.NewUserRepository(pool),
		postgres.NewSessionRepository(pool),
		scratchTenantA, scratchTenantB
}

// newSession builds a valid domain session with a distinct hash per call, since
// sessions_refresh_hash_unique holds.
func newSession(t *testing.T, tenantID, userID string) iam.Session {
	t.Helper()

	created := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	session, err := iam.NewSession(
		iam.SessionID(uuid.NewString()),
		tenant.ID(tenantID),
		iam.UserID(userID),
		iam.RefreshHash("sha256:"+uuid.NewString()),
		created,
		created.Add(30*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("cannot build the domain session: %v", err)
	}
	return session
}

// createUserAndSession persists a user in the given tenant and a session on it.
func createUserAndSession(t *testing.T, transactor *postgres.Transactor, users *postgres.UserRepository, sessions *postgres.SessionRepository, tenantID, email string) (iam.User, iam.Session) {
	t.Helper()
	ctx := context.Background()

	user := newUser(t, tenantID, email)
	if err := transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		return users.Create(ctx, user)
	}); err != nil {
		t.Fatalf("cannot create the user: %v", err)
	}

	session := newSession(t, tenantID, string(user.ID))
	if err := transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		return sessions.Create(ctx, session)
	}); err != nil {
		t.Fatalf("cannot create the session: %v", err)
	}

	return user, session
}

func TestSessionRepository(t *testing.T) {
	transactor, users, sessions, tenantA, tenantB := sessionEnv(t)
	ctx := context.Background()

	t.Run("a session created inside WithTenant reads back by refresh hash", func(t *testing.T) {
		user, created := createUserAndSession(t, transactor, users, sessions, tenantA, "session-create@iam.test")

		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := sessions.ByRefreshHash(ctx, created.RefreshHash)
			if err != nil {
				return err
			}
			if got.ID != created.ID || got.UserID != user.ID {
				t.Errorf("session id/user = %q/%q, want %q/%q", got.ID, got.UserID, created.ID, user.ID)
			}
			if got.TenantID != tenant.ID(tenantA) {
				t.Errorf("TenantID = %q, want %q", got.TenantID, tenantA)
			}
			if got.RevokedAt != nil {
				t.Errorf("a fresh session is already revoked at %v", got.RevokedAt)
			}
			if !got.ExpiresAt.Equal(created.ExpiresAt) {
				t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, created.ExpiresAt)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot read the session back: %v", err)
		}
	})

	t.Run("a session outside WithTenant is refused before the database", func(t *testing.T) {
		user := newUser(t, tenantA, "session-unscoped@iam.test")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return users.Create(ctx, user)
		}); err != nil {
			t.Fatalf("cannot create the user: %v", err)
		}

		err := sessions.Create(ctx, newSession(t, tenantA, string(user.ID)))
		if !errors.Is(err, iam.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want %v", err, iam.ErrTenantScopeMissing)
		}
	})

	// The regression guard for decision 7 of L11: knowing a user UUID from another
	// tenant must not be enough to attach a session to it, because the foreign key
	// on sessions.user_id does not consult row-level security.
	t.Run("a session cannot be built on another tenant's user", func(t *testing.T) {
		foreign := newUser(t, tenantB, "session-foreign@iam.test")
		if err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			return users.Create(ctx, foreign)
		}); err != nil {
			t.Fatalf("cannot create the foreign user: %v", err)
		}

		forged := newSession(t, tenantA, string(foreign.ID))
		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return sessions.Create(ctx, forged)
		})
		if !errors.Is(err, iam.ErrNotFound) {
			t.Fatalf("error = %v, want %v: a foreign user must look like a missing one", err, iam.ErrNotFound)
		}

		// And nothing was written: the guard runs before the insert.
		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, err := sessions.ByRefreshHash(ctx, forged.RefreshHash)
			return err
		})
		if !errors.Is(err, iam.ErrNotFound) {
			t.Fatalf("after the refused insert, ByRefreshHash error = %v, want %v", err, iam.ErrNotFound)
		}
	})

	t.Run("rotation revokes the presented grant and points at its successor", func(t *testing.T) {
		user := newUser(t, tenantA, "session-rotate@iam.test")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return users.Create(ctx, user)
		}); err != nil {
			t.Fatalf("cannot create the user: %v", err)
		}

		original := newSession(t, tenantA, string(user.ID))
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return sessions.Create(ctx, original)
		}); err != nil {
			t.Fatalf("cannot create the original session: %v", err)
		}

		successor := newSession(t, tenantA, string(user.ID))
		rotatedAt := time.Date(2026, 2, 2, 8, 0, 0, 0, time.UTC)
		revoked, err := original.ReplaceWith(successor.ID, rotatedAt)
		if err != nil {
			t.Fatalf("cannot apply the domain transition: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return sessions.Rotate(ctx, revoked, successor)
		}); err != nil {
			t.Fatalf("cannot rotate: %v", err)
		}

		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			// The revoked row stays reachable, which is what makes a replay
			// detectable instead of invisible.
			previous, err := sessions.ByRefreshHash(ctx, original.RefreshHash)
			if err != nil {
				return err
			}
			if previous.RevokedAt == nil {
				t.Error("the rotated session is not revoked")
			}
			if previous.ReplacedBy == nil || *previous.ReplacedBy != successor.ID {
				t.Errorf("ReplacedBy = %v, want %q", previous.ReplacedBy, successor.ID)
			}

			next, err := sessions.ByRefreshHash(ctx, successor.RefreshHash)
			if err != nil {
				return err
			}
			if next.RevokedAt != nil {
				t.Error("the successor is revoked: the rotation killed the live grant too")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot read the rotation back: %v", err)
		}

		// A second rotation of the same grant is the reuse signal.
		another := newSession(t, tenantA, string(user.ID))
		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return sessions.Rotate(ctx, revoked, another)
		})
		if !errors.Is(err, iam.ErrInvalidTransition) {
			t.Fatalf("rotating a spent grant = %v, want %v: reuse must not look like a missing token", err, iam.ErrInvalidTransition)
		}
	})

	t.Run("RevokeChain kills every successor in the chain", func(t *testing.T) {
		user := newUser(t, tenantA, "session-chain@iam.test")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return users.Create(ctx, user)
		}); err != nil {
			t.Fatalf("cannot create the user: %v", err)
		}

		// A three-deep chain: first -> second -> third.
		first := newSession(t, tenantA, string(user.ID))
		second := newSession(t, tenantA, string(user.ID))
		third := newSession(t, tenantA, string(user.ID))

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return sessions.Create(ctx, first)
		}); err != nil {
			t.Fatalf("cannot create the first session: %v", err)
		}

		for _, step := range []struct {
			previous  iam.Session
			successor iam.Session
		}{
			{first, second},
			{second, third},
		} {
			revoked, err := step.previous.ReplaceWith(step.successor.ID, time.Date(2026, 2, 3, 9, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("cannot apply the domain transition: %v", err)
			}
			if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
				return sessions.Rotate(ctx, revoked, step.successor)
			}); err != nil {
				t.Fatalf("cannot rotate: %v", err)
			}
		}

		// Rotation already revoked the first two, so the chain walk only has the
		// leaf left: the count proves it reached the deepest link.
		revokedAt := time.Date(2026, 2, 4, 9, 0, 0, 0, time.UTC)
		var affected int64
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var err error
			affected, err = sessions.RevokeChain(ctx, first.ID, revokedAt)
			return err
		}); err != nil {
			t.Fatalf("cannot revoke the chain: %v", err)
		}
		if affected != 1 {
			t.Fatalf("rows revoked = %d, want 1 (only the leaf was still live)", affected)
		}

		for _, hash := range []iam.RefreshHash{first.RefreshHash, second.RefreshHash, third.RefreshHash} {
			if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
				got, err := sessions.ByRefreshHash(ctx, hash)
				if err != nil {
					return err
				}
				if got.RevokedAt == nil {
					t.Errorf("session %q survived the chain revocation", got.ID)
				}
				return nil
			}); err != nil {
				t.Fatalf("cannot read the chain back: %v", err)
			}
		}
	})

	t.Run("a missing refresh hash is ErrNotFound", func(t *testing.T) {
		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, err := sessions.ByRefreshHash(ctx, iam.RefreshHash("sha256:never-issued"))
			return err
		})
		if !errors.Is(err, iam.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, iam.ErrNotFound)
		}
	})

	t.Run("a session of one tenant is invisible from another", func(t *testing.T) {
		_, inA := createUserAndSession(t, transactor, users, sessions, tenantA, "session-isolation@iam.test")

		err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			_, err := sessions.ByRefreshHash(ctx, inA.RefreshHash)
			return err
		})
		if !errors.Is(err, iam.ErrNotFound) {
			t.Fatalf("error = %v, want %v: row-level security must hide another tenant's session", err, iam.ErrNotFound)
		}
	})
}

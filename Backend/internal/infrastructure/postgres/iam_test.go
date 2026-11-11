package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// scratchTenantA and scratchTenantB are the tenants every IAM test works in.
// New UUIDs per run keep concurrent runs and leftovers apart.
var (
	scratchTenantA = uuid.NewString()
	scratchTenantB = uuid.NewString()
)

// newScratchDatabase creates a throwaway database, returns the two DSNs that
// point at it, and arranges for it to be dropped when the test ends. The admin
// connection stays open until cleanup: cleanups are LIFO, so the drop (and the
// connection it needs) runs after the assertions.
func newScratchDatabase(t *testing.T, ctx context.Context, ownerDSN, appDSN string) (tempOwnerDSN, tempAppDSN string) {
	t.Helper()

	database := "aveniq_iam_" + uuid.NewString()[:8]
	quotedDatabase := pgx.Identifier{database}.Sanitize()

	admin, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("cannot connect to the migrator database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedDatabase); err != nil {
		t.Fatalf("cannot create the scratch database: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quotedDatabase+" WITH (FORCE)"); err != nil {
			t.Logf("could not drop the scratch database %s: %v", database, err)
		}
	})

	return withDatabase(t, ownerDSN, database), withDatabase(t, appDSN, database)
}

// iamEnv builds the scratch environment the IAM tests run against: a throwaway
// database with the real migrations, a pool connecting as the application role
// (the only role RLS applies to), and the adapters under test.
//
// Like TestIsolation, it skips cleanly when the database DSNs are not set, so a
// plain `go test ./...` stays green on a machine with no database.
func iamEnv(t *testing.T) (*pgxpool.Pool, *postgres.Transactor, *postgres.UserRepository, string, string) {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the IAM repository tests")
	}

	ctx := context.Background()

	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)

	seedTenants(t, ctx, tempOwnerDSN)
	tenantA, tenantB := scratchTenantA, scratchTenantB

	pool, err := pgxpool.New(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database as the application role: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return pool, postgres.NewTransactor(pool), postgres.NewUserRepository(pool), tenantA, tenantB
}

// seedTenants inserts the two tenants every IAM test works in.
func seedTenants(t *testing.T, ctx context.Context, ownerDSN string) {
	t.Helper()

	conn, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	for _, subject := range []struct {
		id   string
		slug string
		name string
	}{
		{scratchTenantA, "iam-a", "IAM A"},
		{scratchTenantB, "iam-b", "IAM B"},
	} {
		if _, err := conn.Exec(ctx, `
			INSERT INTO tenants (id, slug, name, status, created_at, updated_at)
			VALUES ($1, $2, $3, 'trial', now(), now())`,
			subject.id, subject.slug, subject.name); err != nil {
			t.Fatalf("cannot insert tenant %s: %v", subject.slug, err)
		}
	}
}

// newUser builds a valid domain user for the given tenant, with a distinct
// UUID when the same email must coexist in two tenants.
func newUser(t *testing.T, tenantID string, email string) iam.User {
	t.Helper()

	subject, err := iam.NewUser(
		iam.UserID(uuid.NewString()),
		tenant.ID(tenantID),
		email,
		"argon2id$v=19$m=65536,t=3,p=4$testhash",
		"Ana Pérez",
		iam.RoleProfessional,
		time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("cannot build the domain user: %v", err)
	}
	return subject
}

func TestUserRepository(t *testing.T) {
	pool, transactor, repo, tenantA, tenantB := iamEnv(t)
	ctx := context.Background()

	t.Run("creating a user inside WithTenant succeeds", func(t *testing.T) {
		subject := newUser(t, tenantA, "create@iam.test")

		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		})
		if err != nil {
			t.Fatalf("cannot create the user inside WithTenant: %v", err)
		}

		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := repo.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.Email != subject.Email || got.Role != subject.Role {
				t.Errorf("read back = %q/%q, want %q/%q", got.Email, got.Role, subject.Email, subject.Role)
			}
			if got.TenantID != subject.TenantID {
				t.Errorf("TenantID = %q, want %q", got.TenantID, subject.TenantID)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot read the user back: %v", err)
		}
	})

	t.Run("the same email can exist in two different tenants", func(t *testing.T) {
		shared := "shared@iam.test"
		inA := newUser(t, tenantA, shared)
		inB := newUser(t, tenantB, shared)

		for _, tc := range []struct {
			tenantID string
			subject  iam.User
		}{
			{tenantA, inA},
			{tenantB, inB},
		} {
			if err := transactor.WithTenant(ctx, tc.tenantID, func(ctx context.Context) error {
				return repo.Create(ctx, tc.subject)
			}); err != nil {
				t.Fatalf("cannot create %q in tenant %s: %v", shared, tc.tenantID, err)
			}
		}
	})

	t.Run("the same email twice in one tenant is a domain conflict", func(t *testing.T) {
		subject := newUser(t, tenantA, "dupe@iam.test")

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		}); err != nil {
			t.Fatalf("cannot create the first user: %v", err)
		}

		duplicate := newUser(t, tenantA, "dupe@iam.test")
		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, duplicate)
		})
		if !errors.Is(err, iam.ErrEmailTaken) {
			t.Fatalf("error = %v, want %v: the application layer must never see a driver error", err, iam.ErrEmailTaken)
		}
	})

	t.Run("reading users returns only the current tenant's rows", func(t *testing.T) {
		onlyA := newUser(t, tenantA, "isolation@iam.test")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, onlyA)
		}); err != nil {
			t.Fatalf("cannot create the user: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			users, err := repo.List(ctx, 100, 0)
			if err != nil {
				return err
			}
			for _, got := range users {
				if got.Email == "shared@iam.test" && got.TenantID != onlyA.TenantID {
					t.Errorf("List returned a row of another tenant: email %q, tenant %q", got.Email, got.TenantID)
				}
			}

			// B's user exists in the database but must be invisible from A.
			if _, err := repo.ByEmail(ctx, "shared@iam.test"); err != nil && !errors.Is(err, iam.ErrNotFound) {
				t.Errorf("ByEmail error = %v, want %v or a tenant-local hit", err, iam.ErrNotFound)
			}
			return nil
		}); err != nil {
			t.Fatalf("isolation check failed: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			got, err := repo.ByEmail(ctx, "shared@iam.test")
			if err != nil {
				return err
			}
			if got.TenantID != tenant.ID(tenantB) {
				t.Errorf("ByEmail returned tenant %q, want %q", got.TenantID, tenantB)
			}
			return nil
		}); err != nil {
			t.Fatalf("cannot read the user from tenant B: %v", err)
		}
	})

	t.Run("a mutation outside WithTenant is impossible", func(t *testing.T) {
		subject := newUser(t, tenantA, "unscoped@iam.test")

		err := repo.Create(ctx, subject)
		if !errors.Is(err, iam.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want %v: without app.tenant_id the policy must fail closed", err, iam.ErrTenantScopeMissing)
		}
	})

	t.Run("a missing user is ErrNotFound", func(t *testing.T) {
		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, err := repo.ByID(ctx, iam.UserID(uuid.NewString()))
			return err
		})
		if !errors.Is(err, iam.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, iam.ErrNotFound)
		}
	})

	t.Run("update persists domain transitions and delete removes the user", func(t *testing.T) {
		subject := newUser(t, tenantA, "lifecycle@iam.test")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		}); err != nil {
			t.Fatalf("cannot create the user: %v", err)
		}

		later := time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)
		deactivated, err := subject.Deactivate(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		retitled, err := deactivated.ChangeRole(iam.RoleAdmin, later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Update(ctx, retitled)
		}); err != nil {
			t.Fatalf("cannot update the user: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := repo.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.IsActive || got.Role != iam.RoleAdmin {
				t.Errorf("after update IsActive/Role = %v/%q, want false/admin", got.IsActive, got.Role)
			}
			if !got.UpdatedAt.Equal(later) {
				t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
			}
			return nil
		}); err != nil {
			t.Fatalf("cannot read the updated user: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Delete(ctx, subject.ID)
		}); err != nil {
			t.Fatalf("cannot delete the user: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, err := repo.ByID(ctx, subject.ID)
			return err
		}); !errors.Is(err, iam.ErrNotFound) {
			t.Fatalf("after delete error = %v, want %v", err, iam.ErrNotFound)
		}
	})

	_ = pool
}

// TestRoleCatalogDrift proves the domain's RoleCode constants and the seeded
// `roles` table never disagree: every domain constant must be a real row, and
// every seeded row must exist in the domain. Either direction of drift would
// make user creation fail at the roles.role_code foreign key — or let a code
// through that the permission matrix (T15) has no entry for.
func TestRoleCatalogDrift(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the role catalog drift test")
	}

	ctx := context.Background()
	tempOwnerDSN, _ := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)

	conn, err := pgx.Connect(ctx, tempOwnerDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(ctx, "SELECT code FROM roles ORDER BY code")
	if err != nil {
		t.Fatalf("cannot read the role catalog: %v", err)
	}
	defer rows.Close()

	seeded := make(map[iam.RoleCode]bool)
	for rows.Next() {
		var code iam.RoleCode
		if err := rows.Scan(&code); err != nil {
			t.Fatalf("cannot scan a role code: %v", err)
		}
		seeded[code] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("cannot read the role catalog: %v", err)
	}

	domain := map[iam.RoleCode]bool{
		iam.RoleOwner:        false,
		iam.RoleAdmin:        false,
		iam.RoleProfessional: false,
		iam.RoleAssistant:    false,
		iam.RoleAccountant:   false,
	}

	for code := range domain {
		if !seeded[code] {
			t.Errorf("domain role %q does not exist in the roles table: user creation would fail at the foreign key", code)
		}
	}
	for code := range seeded {
		if _, known := domain[code]; !known {
			t.Errorf("seeded role %q is unknown to the domain: Valid() would reject a real role", code)
		}
	}
}

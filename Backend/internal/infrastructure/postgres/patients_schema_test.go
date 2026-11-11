package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestPatientsSchemaPrivileges is the check L14 asked for and promised to verify
// instead of trusting: tables created by a later migration must be usable by the
// application role without anyone granting anything, because 0003 left
// ALTER DEFAULT PRIVILEGES behind.
//
// It runs as `aveniq_app`, not as the owner, on purpose. Verified with the owner
// role the test would pass while the application could not write a single row —
// the same class of silent failure as the RLS incident in L7.
func TestPatientsSchemaPrivileges(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the schema tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	app, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = app.Close(ctx) }()

	// One transaction, because app.tenant_id is transactional by design (L6).
	insert := func(ctx context.Context, tenantID, fullName string) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO patients (id, tenant_id, full_name, created_at, updated_at)
				VALUES ($1, $2, $3, now(), now())`,
				uuid.NewString(), tenantID, fullName)
			return err
		})
	}

	t.Run("the application role can write a new table without an explicit grant", func(t *testing.T) {
		if err := insert(ctx, scratchTenantA, "Ana Pérez"); err != nil {
			t.Fatalf("the application role cannot insert into patients: %v — ALTER DEFAULT PRIVILEGES in 0003 did not reach this table", err)
		}
	})

	t.Run("a session sees only its tenant's rows", func(t *testing.T) {
		if err := insert(ctx, scratchTenantA, "Solo de A"); err != nil {
			t.Fatalf("cannot seed tenant A: %v", err)
		}

		count := func(ctx context.Context, tenantID string) int {
			t.Helper()
			var total int
			if err := withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
				return app.QueryRow(ctx, "SELECT count(*) FROM patients").Scan(&total)
			}); err != nil {
				t.Fatalf("cannot count patients for %s: %v", tenantID, err)
			}
			return total
		}

		if got := count(ctx, scratchTenantB); got != 0 {
			t.Fatalf("tenant B sees %d patients of tenant A: the policy is not filtering", got)
		}
		if got := count(ctx, scratchTenantA); got == 0 {
			t.Fatal("tenant A cannot see its own patients")
		}
	})

	t.Run("without a tenant setting the policy fails closed", func(t *testing.T) {
		var total int
		if err := app.QueryRow(ctx, "SELECT count(*) FROM patients").Scan(&total); err != nil {
			t.Fatalf("cannot count without context: %v", err)
		}
		if total != 0 {
			t.Fatalf("a session without app.tenant_id sees %d rows, want 0", total)
		}
	})

	t.Run("writing into another tenant is refused by the policy", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO patients (id, tenant_id, full_name, created_at, updated_at)
				VALUES ($1, $2, $3, now(), now())`,
				uuid.NewString(), scratchTenantB, "Intruso")
			return err
		})

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("error = %v, want SQLSTATE 42501 from the WITH CHECK policy", err)
		}
	})

	// S16's uniqueness rule, verified at the schema instead of assumed: the same
	// person may be a patient in two practices, but the document identifies them
	// once per practice.
	t.Run("a document is unique inside a tenant and shared across tenants", func(t *testing.T) {
		const document = "30111222"

		insertWithDocument := func(ctx context.Context, tenantID string) error {
			return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
				_, err := app.Exec(ctx, `
					INSERT INTO patients (id, tenant_id, full_name, document_type, document_number, created_at, updated_at)
					VALUES ($1, $2, $3, 'dni', $4, now(), now())`,
					uuid.NewString(), tenantID, "Persona Compartida", document)
				return err
			})
		}

		if err := insertWithDocument(ctx, scratchTenantA); err != nil {
			t.Fatalf("cannot insert the first patient with document: %v", err)
		}

		duplicate := insertWithDocument(ctx, scratchTenantA)
		var pgErr *pgconn.PgError
		if !errors.As(duplicate, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("the same document twice in one tenant = %v, want SQLSTATE 23505", duplicate)
		}

		if err := insertWithDocument(ctx, scratchTenantB); err != nil {
			t.Fatalf("the same document in another tenant must be allowed: %v", err)
		}
	})
}

// withTenantSetting runs fn in a transaction that carries app.tenant_id, which is
// the only form row-level security reads.
func withTenantSetting(ctx context.Context, conn *pgx.Conn, tenantID string, fn func(ctx context.Context) error) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return err
	}
	if err := fn(ctx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

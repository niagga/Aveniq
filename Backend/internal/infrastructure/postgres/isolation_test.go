// Package postgres_test holds the tests that protect the guarantees the
// PostgreSQL adapter depends on. They exercise raw SQL on purpose: the isolation
// they verify is a database guarantee, not a Go one, and it must hold even if
// every line of application code is wrong.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver used by goose
	"github.com/pressly/goose/v3"
)

// migrationsDir is resolved against the package directory, which is the working
// directory `go test` uses.
const migrationsDir = "../../../migrations"

// rlsViolationCode is PostgreSQL's SQLSTATE for "insufficient privilege", which
// is what a WITH CHECK policy raises when a row lands in the wrong tenant.
const rlsViolationCode = "42501"

// TestIsolation proves that row-level security actually isolates tenants.
//
// It builds a throwaway database, applies the real migrations, seeds two tenants
// and asserts what each one can see and write. It needs two connections because
// the guarantee only holds for a role that is neither superuser nor table owner:
//
//	TEST_DATABASE_URL      migrator/owner DSN, needs CREATEDB
//	TEST_APP_DATABASE_URL  application role DSN (aveniq_app), non-superuser
//
// Without both, the test skips so that a plain `go test ./...` stays green on a
// machine with no database.
func TestIsolation(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the isolation test")
	}

	ctx := context.Background()

	// A short random suffix keeps concurrent runs and leftovers apart.
	database := "aveniq_iso_" + uuid.NewString()[:8]
	quotedDatabase := pgx.Identifier{database}.Sanitize()

	admin, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("cannot connect to the migrator database: %v", err)
	}
	// Registered first, so it runs last: cleanups are LIFO and the connection
	// must still be open when the scratch database is dropped.
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quotedDatabase); err != nil {
		t.Fatalf("cannot create the scratch database: %v", err)
	}

	// Dropping is best-effort but unconditional: a failed assertion must not
	// leave a database behind.
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quotedDatabase+" WITH (FORCE)"); err != nil {
			t.Logf("could not drop the scratch database %s: %v", database, err)
		}
	})

	tempOwnerDSN := withDatabase(t, ownerDSN, database)
	tempAppDSN := withDatabase(t, appDSN, database)

	migrate(t, tempOwnerDSN)

	tenantA, tenantB := seed(t, ctx, tempOwnerDSN)

	app, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = app.Close(ctx) }()

	t.Run("a tenant sees only its own users", func(t *testing.T) {
		got := countVisible(t, ctx, app, "users", &tenantA)
		if got != 1 {
			t.Errorf("tenant A sees %d users, want 1", got)
		}
	})

	t.Run("another tenant sees only its own users", func(t *testing.T) {
		got := countVisible(t, ctx, app, "users", &tenantB)
		if got != 1 {
			t.Errorf("tenant B sees %d users, want 1", got)
		}
	})

	t.Run("without tenant context nothing is visible", func(t *testing.T) {
		got := countVisible(t, ctx, app, "users", nil)
		if got != 0 {
			t.Errorf("a session without app.tenant_id sees %d users, want 0: "+
				"the policy is not failing closed", got)
		}
	})

	t.Run("an empty tenant context sees nothing", func(t *testing.T) {
		empty := ""
		got := countVisible(t, ctx, app, "users", &empty)
		if got != 0 {
			t.Errorf("a session with an empty app.tenant_id sees %d users, want 0", got)
		}
	})

	t.Run("sessions are isolated too", func(t *testing.T) {
		if got := countVisible(t, ctx, app, "sessions", &tenantA); got != 1 {
			t.Errorf("tenant A sees %d sessions, want 1", got)
		}
		if got := countVisible(t, ctx, app, "sessions", nil); got != 0 {
			t.Errorf("a session without app.tenant_id sees %d sessions, want 0", got)
		}
	})

	t.Run("a row cannot be written into another tenant", func(t *testing.T) {
		tx, err := app.Begin(ctx)
		if err != nil {
			t.Fatalf("cannot begin the transaction: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantA); err != nil {
			t.Fatalf("cannot set the tenant context: %v", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, created_at, updated_at)
			VALUES ($1, $2, 'intruder@iso.test', 'hash', 'Intruder', 'owner', now(), now())`,
			uuid.NewString(), tenantB)
		if err == nil {
			t.Fatal("a row was written into another tenant: the policy is missing WITH CHECK")
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != rlsViolationCode {
			t.Fatalf("error = %v, want SQLSTATE %s from the row-level security policy", err, rlsViolationCode)
		}
	})
}

// migrate applies the real migrations to the scratch database, so the test
// validates the schema that actually ships instead of a copy of it.
func migrate(t *testing.T, dsn string) {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("cannot open the migration connection: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("cannot set the goose dialect: %v", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		t.Fatalf("cannot apply the migrations: %v", err)
	}
}

// seed creates two tenants with a user and a session each, and returns the
// tenant identifiers. Writes go through the tenant context as well, because the
// policies apply to the migrator too once FORCE is in place.
func seed(t *testing.T, ctx context.Context, dsn string) (tenantA, tenantB string) {
	t.Helper()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	tenantA = uuid.NewString()
	tenantB = uuid.NewString()

	for _, tenant := range []struct {
		id, slug, name, email string
	}{
		{tenantA, "iso-a", "Isolation A", "a@iso.test"},
		{tenantB, "iso-b", "Isolation B", "b@iso.test"},
	} {
		if _, err := conn.Exec(ctx, `
			INSERT INTO tenants (id, slug, name, status, created_at, updated_at)
			VALUES ($1, $2, $3, 'trial', now(), now())`,
			tenant.id, tenant.slug, tenant.name); err != nil {
			t.Fatalf("cannot insert tenant %s: %v", tenant.slug, err)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatalf("cannot begin the seed transaction: %v", err)
		}

		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant.id); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("cannot set the tenant context: %v", err)
		}

		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, created_at, updated_at)
			VALUES ($1, $2, $3, 'hash', $4, 'owner', now(), now())`,
			userID, tenant.id, tenant.email, tenant.name); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("cannot insert the user of %s: %v", tenant.slug, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO sessions (id, tenant_id, user_id, refresh_hash, created_at, expires_at)
			VALUES ($1, $2, $3, $4, now(), now() + interval '30 days')`,
			uuid.NewString(), tenant.id, userID, "refresh-"+userID); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("cannot insert the session of %s: %v", tenant.slug, err)
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("cannot commit the seed of %s: %v", tenant.slug, err)
		}
	}

	return tenantA, tenantB
}

// countVisible counts the rows of a table inside a transaction carrying the given
// tenant context. A nil context means "never set the setting", which models a
// request that reached the database without being scoped.
//
// set_config(..., true) is the transactional form of SET LOCAL and, unlike SET,
// accepts a parameter.
func countVisible(t *testing.T, ctx context.Context, conn *pgx.Conn, table string, tenantID *string) int {
	t.Helper()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("cannot begin the transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if tenantID != nil {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", *tenantID); err != nil {
			t.Fatalf("cannot set the tenant context: %v", err)
		}
	}

	var count int
	// The table name is a package constant supplied by the test itself, never input.
	if err := tx.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count); err != nil {
		t.Fatalf("cannot count rows of %s: %v", table, err)
	}
	return count
}

// withDatabase returns the same DSN pointing at another database.
func withDatabase(t *testing.T, dsn, database string) string {
	t.Helper()

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("cannot parse the DSN: %v", err)
	}
	parsed.Path = "/" + database
	return parsed.String()
}

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

// TestAuditLogAppendOnly is T26's structural guarantee: the application records the
// trail and reads it, and cannot rewrite or erase it.
//
// The policy on audit_log is FOR ALL, so without the REVOKE of 0008 an application bug
// could delete the trail it is supposed to leave. Privileges are the only thing that
// makes "append-only" true rather than intended.
func TestAuditLogAppendOnly(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the schema tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)
	_, users := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

	app, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = app.Close(ctx) }()

	expectCode := func(t *testing.T, err error, code, what string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("%s: error = %v, want SQLSTATE %s", what, err, code)
		}
	}

	insert := func(ctx context.Context, tenantID, actorID string) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO audit_log (tenant_id, actor_id, action, entity, entity_id, metadata, created_at)
				VALUES ($1, $2, 'clinical.read', 'clinical_record', $3, '{}'::jsonb, now())`,
				tenantID, actorID, uuid.NewString())
			return err
		})
	}

	t.Run("the application records an event inside the tenant transaction", func(t *testing.T) {
		if err := insert(ctx, scratchTenantA, users[scratchTenantA]); err != nil {
			t.Fatalf("cannot write an audit entry: %v — this is the path clinical reads use", err)
		}
	})

	t.Run("the application reads its own trail and not another practice's", func(t *testing.T) {
		var own, foreign int
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			return app.QueryRow(ctx, "SELECT count(*) FROM audit_log").Scan(&own)
		}); err != nil {
			t.Fatalf("cannot count for A: %v", err)
		}
		if own == 0 {
			t.Fatal("tenant A cannot read the entry it just wrote")
		}

		if err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			return app.QueryRow(ctx, "SELECT count(*) FROM audit_log").Scan(&foreign)
		}); err != nil {
			t.Fatalf("cannot count for B: %v", err)
		}
		if foreign != 0 {
			t.Fatalf("tenant B reads %d audit rows of tenant A", foreign)
		}
	})

	t.Run("the application cannot rewrite the trail", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, "UPDATE audit_log SET action = 'clinical.read.edited'")
			return err
		})
		expectCode(t, err, "42501", "updating the audit log")
	})

	t.Run("the application cannot erase the trail", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, "DELETE FROM audit_log")
			return err
		})
		expectCode(t, err, "42501", "deleting from the audit log")
	})
}

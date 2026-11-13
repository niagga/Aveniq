package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestScheduleSchema(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the schema tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)
	patients, users := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

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

	insert := func(ctx context.Context, tenantID, professionalID, patientID string, start time.Time) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO appointments (
					id, tenant_id, professional_user_id, patient_id,
					starts_at, ends_at, status, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, 'scheduled', now(), now())`,
				uuid.NewString(), tenantID, professionalID, patientID, start, start.Add(45*time.Minute))
			return err
		})
	}

	start := time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)

	t.Run("the application role can write appointments", func(t *testing.T) {
		if err := insert(ctx, scratchTenantA, users[scratchTenantA], patients[scratchTenantA], start); err != nil {
			t.Fatalf("cannot insert an appointment: %v — default privileges did not reach 0009", err)
		}
	})

	t.Run("overlapping live slots of the same professional are refused", func(t *testing.T) {
		err := insert(ctx, scratchTenantA, users[scratchTenantA], patients[scratchTenantA], start.Add(15*time.Minute))
		expectCode(t, err, "23P01", "overlapping appointment")
	})

	t.Run("a cancelled slot frees the interval", func(t *testing.T) {
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE appointments SET status = 'cancelled' WHERE tenant_id = $1`, scratchTenantA)
			return err
		}); err != nil {
			t.Fatalf("cannot cancel: %v", err)
		}
		if err := insert(ctx, scratchTenantA, users[scratchTenantA], patients[scratchTenantA], start); err != nil {
			t.Fatalf("reusing a cancelled slot: %v", err)
		}
	})

	t.Run("a slot cannot hang off another practice's patient", func(t *testing.T) {
		err := insert(ctx, scratchTenantA, users[scratchTenantA], patients[scratchTenantB], start.Add(3*time.Hour))
		expectCode(t, err, "23503", "foreign patient")
	})

	t.Run("a session sees only its own appointments", func(t *testing.T) {
		var total int
		if err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			return app.QueryRow(ctx, "SELECT count(*) FROM appointments").Scan(&total)
		}); err != nil {
			t.Fatalf("count B: %v", err)
		}
		if total != 0 {
			t.Fatalf("tenant B sees %d appointments of A", total)
		}
	})

	t.Run("tenants have a timezone", func(t *testing.T) {
		var zone string
		if err := app.QueryRow(ctx, "SELECT timezone FROM tenants WHERE id = $1", scratchTenantA).Scan(&zone); err != nil {
			t.Fatalf("timezone: %v", err)
		}
		if zone != "America/Argentina/Buenos_Aires" {
			t.Fatalf("timezone = %q", zone)
		}
	})
}

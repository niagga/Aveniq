package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestPatientCurrencyHasNoDefaultAfter0017 proves 0017 does what it says: the billing
// currency is no longer defaulted, so a patient written without one is refused, and its
// Down restores the ARS default that 0016 introduced. It runs on a throwaway database.
func TestPatientCurrencyHasNoDefaultAfter0017(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the currency migration tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	if got := gooseVersion(t, tempOwnerDSN); got != 17 {
		t.Fatalf("after up: goose version = %d, want 17", got)
	}

	owner, err := pgx.Connect(ctx, tempOwnerDSN)
	if err != nil {
		t.Fatalf("cannot connect as the owner: %v", err)
	}
	defer func() { _ = owner.Close(ctx) }()

	app, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = app.Close(ctx) }()

	columnDefault := func() string {
		t.Helper()
		var value *string
		if err := owner.QueryRow(ctx, `
			SELECT column_default FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'patients' AND column_name = 'billing_currency'`,
		).Scan(&value); err != nil {
			t.Fatalf("cannot read the column default: %v", err)
		}
		if value == nil {
			return ""
		}
		return *value
	}

	t.Run("the billing currency has no default", func(t *testing.T) {
		if got := columnDefault(); got != "" {
			t.Fatalf("column default = %q, want none: the currency must be chosen by hand", got)
		}
	})

	t.Run("a patient written without a currency is refused", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO patients (id, tenant_id, full_name, country, created_at, updated_at)
				VALUES ($1, $2, 'Sin moneda', 'AR', now(), now())`,
				uuid.NewString(), scratchTenantA)
			return err
		})

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23502" {
			t.Fatalf("error = %v, want SQLSTATE 23502 not_null_violation on billing_currency", err)
		}
	})

	t.Run("0017 down restores the ARS default", func(t *testing.T) {
		migrateDown(t, tempOwnerDSN)
		if got := gooseVersion(t, tempOwnerDSN); got != 16 {
			t.Fatalf("after 0017 down: goose version = %d, want 16", got)
		}
		if got := columnDefault(); !strings.Contains(got, "'ARS'") {
			t.Fatalf("column default = %q, want 'ARS' restored", got)
		}
	})
}

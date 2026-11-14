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

// TestBillingSchema proves the three rules 0014 puts in the schema: numbers are
// consecutive and never reused, an issued voucher is final, and nothing is deleted.
// It also proves the practice boundary holds for vouchers and their counter.
func TestBillingSchema(t *testing.T) {
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

	ptr := func(s string) *string { return &s }

	// next bumps the practice's counter inside its own transaction, the way the issue use
	// case will, and returns the number the new voucher takes.
	next := func(ctx context.Context, tenantID string) (int, error) {
		var n int
		err := withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			return app.QueryRow(ctx, `
				INSERT INTO voucher_counters (tenant_id, last_number) VALUES ($1, 1)
				ON CONFLICT (tenant_id) DO UPDATE SET last_number = voucher_counters.last_number + 1
				RETURNING last_number`, tenantID).Scan(&n)
		})
		return n, err
	}

	insert := func(ctx context.Context, tenantID string, number int, patientID, buyerName *string, concept string) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO vouchers (
					id, tenant_id, number, patient_id, concept, amount_minor, currency,
					practice_name, buyer_name, issued_by, issued_at, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, 150000, 'ARS', 'Consultorio Test', $6, $7, now(), now(), now())`,
				uuid.NewString(), tenantID, number, patientID, concept, buyerName, users[tenantID])
			return err
		})
	}

	// void moves an issued voucher to voided and reports how many rows the viewer could touch.
	void := func(ctx context.Context, viewerTenant, tenantID string, number int, reason string) (int64, error) {
		var affected int64
		err := withTenantSetting(ctx, app, viewerTenant, func(ctx context.Context) error {
			tag, err := app.Exec(ctx, `
				UPDATE vouchers
				SET status = 'voided', voided_by = $3, voided_at = now(), void_reason = $4, updated_at = now()
				WHERE tenant_id = $1 AND number = $2`, tenantID, number, users[tenantID], reason)
			affected = tag.RowsAffected()
			return err
		})
		return affected, err
	}

	t.Run("the application role can issue, and the counter moves in the same transaction", func(t *testing.T) {
		n, err := next(ctx, scratchTenantA)
		if err != nil {
			t.Fatalf("cannot bump the counter: %v — default privileges did not reach 0014", err)
		}
		if n != 1 {
			t.Fatalf("first number = %d, want 1", n)
		}
		if err := insert(ctx, scratchTenantA, n, nil, nil, "Consulta suelta"); err != nil {
			t.Fatalf("cannot issue a loose charge: %v", err)
		}
	})

	t.Run("a failed issue leaves no gap in the numbering", func(t *testing.T) {
		before, err := next(ctx, scratchTenantA)
		if err != nil {
			t.Fatalf("bump before the failed issue: %v", err)
		}
		_ = withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			if _, err := app.Exec(ctx, `
				UPDATE voucher_counters SET last_number = last_number + 1 WHERE tenant_id = $1`, scratchTenantA); err != nil {
				return err
			}
			return errors.New("simulated failure after the counter moved")
		})
		after, err := next(ctx, scratchTenantA)
		if err != nil {
			t.Fatalf("bump after the failed issue: %v", err)
		}
		if after != before+1 {
			t.Fatalf("after a rolled-back issue the next number = %d, want %d", after, before+1)
		}
	})

	t.Run("a voucher for a patient must carry the buyer snapshot", func(t *testing.T) {
		err := insert(ctx, scratchTenantA, 10, ptr(patients[scratchTenantA]), nil, "Sesión")
		expectCode(t, err, "23514", "patient voucher without a snapshot")

		if err := insert(ctx, scratchTenantA, 11, ptr(patients[scratchTenantA]), ptr("Ana Pérez"), "Sesión"); err != nil {
			t.Fatalf("cannot issue a patient voucher with its snapshot: %v", err)
		}
	})

	t.Run("a loose charge carries no buyer snapshot", func(t *testing.T) {
		err := insert(ctx, scratchTenantA, 12, nil, ptr("Ana Pérez"), "Cargo suelto")
		expectCode(t, err, "23514", "loose charge with a buyer")
	})

	t.Run("a number is never reused within a practice", func(t *testing.T) {
		err := insert(ctx, scratchTenantA, 1, nil, nil, "Repetido")
		expectCode(t, err, "23505", "duplicate number in the same practice")
	})

	t.Run("the same number is free in another practice", func(t *testing.T) {
		if err := insert(ctx, scratchTenantB, 1, nil, nil, "Consulta suelta"); err != nil {
			t.Fatalf("number 1 should be free in practice B: %v", err)
		}
	})

	t.Run("a voucher cannot point at a patient of another practice", func(t *testing.T) {
		err := insert(ctx, scratchTenantA, 13, ptr(patients[scratchTenantB]), ptr("Ajena"), "Sesión")
		expectCode(t, err, "23503", "foreign patient")
	})

	t.Run("an issued voucher cannot change", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE vouchers SET amount_minor = 999 WHERE tenant_id = $1 AND number = 1`, scratchTenantA)
			return err
		})
		expectCode(t, err, "23001", "amount change on an issued voucher")
	})

	t.Run("a void needs a reason and is final", func(t *testing.T) {
		affected, err := void(ctx, scratchTenantA, scratchTenantA, 1, "   ")
		expectCode(t, err, "23514", "void without a reason")
		if affected != 0 {
			t.Fatalf("a refused void touched %d rows", affected)
		}

		affected, err = void(ctx, scratchTenantA, scratchTenantA, 1, "Error de carga")
		if err != nil || affected != 1 {
			t.Fatalf("void with a reason: affected=%d err=%v, want 1 row", affected, err)
		}

		err = withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE vouchers SET void_reason = 'otra' WHERE tenant_id = $1 AND number = 1`, scratchTenantA)
			return err
		})
		expectCode(t, err, "23001", "edit after the void")

		_, err = void(ctx, scratchTenantA, scratchTenantA, 1, "Otra vez")
		expectCode(t, err, "23001", "second void")
	})

	t.Run("vouchers are never deleted", func(t *testing.T) {
		if err := insert(ctx, scratchTenantA, 20, nil, nil, "Para borrar"); err != nil {
			t.Fatalf("setup: %v", err)
		}
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `DELETE FROM vouchers WHERE tenant_id = $1 AND number = 20`, scratchTenantA)
			return err
		})
		expectCode(t, err, "23001", "delete of an issued voucher")
	})

	t.Run("a practice cannot see or void another practice's vouchers", func(t *testing.T) {
		var visible int
		err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			return app.QueryRow(ctx, `SELECT count(*) FROM vouchers WHERE tenant_id = $1`, scratchTenantA).Scan(&visible)
		})
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if visible != 0 {
			t.Fatalf("practice B sees %d vouchers of practice A", visible)
		}

		affected, err := void(ctx, scratchTenantB, scratchTenantA, 20, "Ajena")
		if err != nil || affected != 0 {
			t.Fatalf("practice B voided A's voucher: affected=%d err=%v, want 0 rows and no error", affected, err)
		}

		var counters int
		err = withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			return app.QueryRow(ctx, `SELECT count(*) FROM voucher_counters WHERE tenant_id = $1`, scratchTenantA).Scan(&counters)
		})
		if err != nil {
			t.Fatalf("count counters: %v", err)
		}
		if counters != 0 {
			t.Fatalf("practice B sees A's counter")
		}
	})
}

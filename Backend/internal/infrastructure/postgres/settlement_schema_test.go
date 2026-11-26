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

// TestSettlementSchema proves the rules 0021 puts in the schema: a session is settled once, an
// issued settlement is frozen and never deleted, and a session is released only by the void of
// its settlement, after which it is final. It also proves the practice boundary holds.
func TestSettlementSchema(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the settlement schema tests")
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

	// The mutual, a patient it covers, and one attended session of that patient, all in practice A.
	insurerA := uuid.NewString()
	coveredA := uuid.NewString()
	sessionA := uuid.NewString()
	start := time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)
	if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
		if _, err := app.Exec(ctx, `
			INSERT INTO insurers (id, tenant_id, name, tax_id, created_at, updated_at)
			VALUES ($1, $2, 'OSDE', '30711223334', now(), now())`, insurerA, scratchTenantA); err != nil {
			return err
		}
		if schemaPatientsHaveClientID(ctx, app) {
			clientID, err := insertClientApp(ctx, app, scratchTenantA, "Ana Mutual", "AR")
			if err != nil {
				return err
			}
			if _, err := app.Exec(ctx, `
				INSERT INTO patients (id, tenant_id, client_id, country, full_name, coverage_kind, insurer_id, member_number, created_at, updated_at)
				VALUES ($1, $2, $3, 'AR', 'Ana Mutual', 'insurer', $4, 'AF-1', now(), now())`, coveredA, scratchTenantA, clientID, insurerA); err != nil {
				return err
			}
		} else if _, err := app.Exec(ctx, `
			INSERT INTO patients (id, tenant_id, country, full_name, coverage_kind, insurer_id, member_number, created_at, updated_at)
			VALUES ($1, $2, 'AR', 'Ana Mutual', 'insurer', $3, 'AF-1', now(), now())`, coveredA, scratchTenantA, insurerA); err != nil {
			return err
		}
		_, err := app.Exec(ctx, `
			INSERT INTO appointments (id, tenant_id, professional_user_id, patient_id, starts_at, ends_at, status, modality, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'attended', 'presencial', now(), now())`,
			sessionA, scratchTenantA, users[scratchTenantA], coveredA, start, start.Add(45*time.Minute))
		return err
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// insertSettlement writes an issued settlement of the mutual, as the application does.
	insertSettlement := func(id, periodStart, periodEnd, taxID string) error {
		return withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO settlements (id, tenant_id, insurer_id, insurer_name, insurer_tax_id, practice_name,
					period_start, period_end, status, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, 'OSDE', $4, 'Consultorio Test', $5::date, $6::date, 'issued', $7, now(), now())`,
				id, scratchTenantA, insurerA, taxID, periodStart, periodEnd, users[scratchTenantA])
			return err
		})
	}

	// insertItem puts the session in the settlement, valued at the mutual's share.
	insertItem := func(settlementID, appointmentID string) error {
		return withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO settlement_items (tenant_id, settlement_id, appointment_id, patient_id, patient_name,
					member_number, professional_user_id, professional_name, modality, starts_at, currency, amount_minor)
				VALUES ($1, $2, $3, $4, 'Ana Mutual', 'AF-1', $5, 'Demo Profesional', 'presencial', $6, 'ARS', 800000)`,
				scratchTenantA, settlementID, appointmentID, coveredA, users[scratchTenantA], start)
			return err
		})
	}

	// void voids a settlement and reports how many rows the update touched.
	void := func(settlementID, reason string) (int64, error) {
		var affected int64
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			tag, err := app.Exec(ctx, `
				UPDATE settlements
				   SET status = 'voided', voided_by = $2, voided_at = now(), void_reason = $3, updated_at = now()
				 WHERE id = $1`, settlementID, users[scratchTenantA], reason)
			affected = tag.RowsAffected()
			return err
		})
		return affected, err
	}

	// release sets released_at on the session's item of a settlement.
	release := func(settlementID, appointmentID string) error {
		return withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				UPDATE settlement_items SET released_at = now()
				 WHERE tenant_id = $1 AND settlement_id = $2 AND appointment_id = $3`,
				scratchTenantA, settlementID, appointmentID)
			return err
		})
	}

	first := uuid.NewString()
	if err := insertSettlement(first, "2026-10-01", "2026-10-31", "30711223334"); err != nil {
		t.Fatalf("setup: issue the October settlement: %v", err)
	}

	t.Run("a session is settled by one active settlement at a time", func(t *testing.T) {
		if err := insertItem(first, sessionA); err != nil {
			t.Fatalf("settle the session: %v", err)
		}
		second := uuid.NewString()
		if err := insertSettlement(second, "2026-10-01", "2026-10-31", "30711223334"); err != nil {
			t.Fatalf("issue a second settlement of the month: %v", err)
		}
		expectCode(t, insertItem(second, sessionA), "23505", "the session in a second active settlement")
	})

	t.Run("an issued settlement is frozen and never deleted", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE settlements SET practice_name = 'Otro nombre' WHERE id = $1`, first)
			return err
		})
		expectCode(t, err, "23001", "rename the practice of an issued settlement")

		err = withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE settlement_items SET amount_minor = 1 WHERE settlement_id = $1`, first)
			return err
		})
		expectCode(t, err, "23001", "change the amount of a settled session")

		expectCode(t, release(first, sessionA), "23001", "release a session while its settlement stands")

		err = withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE settlement_items SET copago_minor = 1 WHERE settlement_id = $1`, first)
			return err
		})
		expectCode(t, err, "23001", "change the copago of a settled session")

		err = withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `DELETE FROM settlements WHERE id = $1`, first)
			return err
		})
		expectCode(t, err, "23001", "delete an issued settlement")
	})

	t.Run("a void needs a reason", func(t *testing.T) {
		affected, err := void(first, "   ")
		expectCode(t, err, "23514", "void without a reason")
		if affected != 0 {
			t.Fatalf("a refused void touched %d rows", affected)
		}
	})

	t.Run("a void releases the sessions, which can then be settled again", func(t *testing.T) {
		affected, err := void(first, "Error de carga")
		if err != nil || affected != 1 {
			t.Fatalf("void with a reason: affected=%d err=%v, want 1 row", affected, err)
		}
		if err := release(first, sessionA); err != nil {
			t.Fatalf("release the session of the voided settlement: %v", err)
		}
		again := uuid.NewString()
		if err := insertSettlement(again, "2026-10-01", "2026-10-31", "30711223334"); err != nil {
			t.Fatalf("issue again: %v", err)
		}
		if err := insertItem(again, sessionA); err != nil {
			t.Fatalf("settle the released session again: %v", err)
		}
	})

	t.Run("a released session and a voided settlement are final", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE settlement_items SET released_at = NULL WHERE settlement_id = $1`, first)
			return err
		})
		expectCode(t, err, "23001", "reopen a released session")

		err = withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE settlements SET void_reason = 'Otro motivo' WHERE id = $1`, first)
			return err
		})
		expectCode(t, err, "23001", "change the void of a voided settlement")
	})

	t.Run("a settlement is one whole month, and the mutual needs its CUIT", func(t *testing.T) {
		expectCode(t, insertSettlement(uuid.NewString(), "2026-10-02", "2026-11-01", "30711223334"), "23514", "a period that starts mid-month")
		expectCode(t, insertSettlement(uuid.NewString(), "2026-10-01", "2026-10-31", "3071"), "23514", "a CUIT of the wrong shape")
	})

	t.Run("the practice boundary holds for settlements", func(t *testing.T) {
		var seen int
		if err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			return app.QueryRow(ctx, `SELECT count(*) FROM settlements WHERE id = $1`, first).Scan(&seen)
		}); err != nil {
			t.Fatalf("practice B reads settlements: %v", err)
		}
		if seen != 0 {
			t.Fatalf("practice B sees %d settlements of practice A, want 0", seen)
		}

		err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO settlements (id, tenant_id, insurer_id, insurer_name, insurer_tax_id, practice_name,
					period_start, period_end, status, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, 'OSDE', '30711223334', 'Otra', '2026-10-01', '2026-10-31', 'issued', $4, now(), now())`,
				uuid.NewString(), scratchTenantA, insurerA, users[scratchTenantB])
			return err
		})
		expectCode(t, err, "42501", "practice B writes a settlement under practice A")
	})
}

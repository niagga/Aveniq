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

// TestVoucherClientMigrationRollsBack proves 0027 does what it says: every voucher with
// a patient is linked to the client that pays for it, the link follows patient_id exactly
// (a loose charge has neither), the client must belong to the practice, the link is frozen
// with the rest of the issued voucher, and its Down removes the column while the buyer
// snapshot stays frozen the way 0020 left it. It runs on a throwaway database.
func TestVoucherClientMigrationRollsBack(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the voucher migration tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)

	// The voucher exists before the migration, so the backfill and the untouched snapshot
	// both have something to prove.
	migrateUpTo(t, tempOwnerDSN, 26)
	if got := gooseVersion(t, tempOwnerDSN); got != 26 {
		t.Fatalf("after up: goose version = %d, want 26", got)
	}
	seedTenants(t, ctx, tempOwnerDSN)
	patients, users := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

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

	expectCode := func(t *testing.T, err error, code, what string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("%s: error = %v, want SQLSTATE %s", what, err, code)
		}
	}

	ptr := func(s string) *string { return &s }

	// Issues a voucher the way 0014 stamped the buyer: snapshot taken from the patient
	// row that existed at issue. Before the migration, no client column exists.
	issueOld := func(t *testing.T, tenantID string, patientID, buyerName *string, number int) error {
		t.Helper()
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO vouchers (
					id, tenant_id, number, patient_id, concept, amount_minor, currency,
					practice_name, buyer_name, coverage_kind, issued_by, issued_at, created_at, updated_at
				) VALUES ($1, $2, $3, $4, 'Sesión', 150000, 'ARS', 'Consultorio Test', $5,
					CASE WHEN $4::uuid IS NULL THEN NULL ELSE 'private' END,
					$6, now(), now(), now())`,
				uuid.NewString(), tenantID, number, patientID, buyerName, users[tenantID])
			return err
		})
	}

	// Issues a voucher after the migration, with its client link, the way the application does.
	issueNew := func(t *testing.T, tenantID, patientID, clientID, buyerName string, number int) error {
		t.Helper()
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO vouchers (
					id, tenant_id, number, patient_id, client_id, concept, amount_minor, currency,
					practice_name, buyer_name, coverage_kind, issued_by, issued_at, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, 'Sesión', 150000, 'ARS', 'Consultorio Test', $6,
					'private', $7, now(), now(), now())`,
				uuid.NewString(), tenantID, number, patientID, clientID, buyerName, users[tenantID])
			return err
		})
	}

	// payerOf reads the payer 0026 built for a patient.
	payerOf := func(t *testing.T, tenantID, patientID string) string {
		t.Helper()
		var clientID string
		if err := withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			return app.QueryRow(ctx, `SELECT client_id::text FROM patients WHERE id = $1`, patientID).Scan(&clientID)
		}); err != nil {
			t.Fatalf("cannot read the payer of %s: %v", patientID, err)
		}
		return clientID
	}

	// Loose charge: no patient, no snapshot.
	if err := issueOld(t, scratchTenantA, nil, nil, 1); err != nil {
		t.Fatalf("setup: loose charge: %v", err)
	}
	// Patient voucher: the snapshot the patient record carried at issue.
	if err := issueOld(t, scratchTenantA, ptr(patients[scratchTenantA]), ptr("Paciente de prueba"), 2); err != nil {
		t.Fatalf("setup: patient voucher: %v", err)
	}
	// The other practice's patient voucher: the backfill must not mix tenants.
	if err := issueOld(t, scratchTenantB, ptr(patients[scratchTenantB]), ptr("Paciente de prueba"), 1); err != nil {
		t.Fatalf("setup: patient voucher of practice B: %v", err)
	}

	migrateUpTo(t, tempOwnerDSN, 27)
	if got := gooseVersion(t, tempOwnerDSN); got != 27 {
		t.Fatalf("after the migration: goose version = %d, want 27", got)
	}

	t.Run("0027 links every patient voucher to its client and keeps the snapshot", func(t *testing.T) {
		var buyer *string
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			var clientID *string
			if err := app.QueryRow(ctx, `SELECT client_id::text, buyer_name FROM vouchers WHERE tenant_id = $1 AND number = 1`, scratchTenantA).Scan(&clientID, &buyer); err != nil {
				return err
			}
			if clientID != nil {
				t.Errorf("a loose charge should have no client, got %v", *clientID)
			}
			return nil
		}); err != nil {
			t.Fatalf("cannot read the loose charge: %v", err)
		}
		// The loose charge was issued with nobody paying through it, and the backfill is not
		// allowed to invent a buyer for it either.
		if buyer != nil {
			t.Fatalf("the loose charge gained a buyer: %q", *buyer)
		}

		wantClient := payerOf(t, scratchTenantA, patients[scratchTenantA])
		var gotClient string
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			return app.QueryRow(ctx, `SELECT client_id::text FROM vouchers WHERE tenant_id = $1 AND number = 2`, scratchTenantA).Scan(&gotClient)
		}); err != nil {
			t.Fatalf("cannot read the patient voucher: %v", err)
		}
		if gotClient != wantClient {
			t.Fatalf("backfill = %q, want the payer of the patient %q", gotClient, wantClient)
		}

		var otherClient string
		if err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			return app.QueryRow(ctx, `SELECT client_id::text FROM vouchers WHERE tenant_id = $1 AND number = 1`, scratchTenantB).Scan(&otherClient)
		}); err != nil {
			t.Fatalf("cannot read practice B's voucher: %v", err)
		}
		if otherClient == wantClient {
			t.Fatal("practice B's voucher was backfilled with practice A's client")
		}
	})

	t.Run("a loose charge carries no client", func(t *testing.T) {
		// Read before opening the tenant scope: the helper sets the tenant on its own
		// transaction, and nesting it inside another one loses the setting.
		payer := payerOf(t, scratchTenantA, patients[scratchTenantA])
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO vouchers (
					id, tenant_id, number, client_id, concept, amount_minor, currency,
					practice_name, issued_by, issued_at, created_at, updated_at
				) VALUES ($1, $2, 3, $3, 'Cargo suelto', 150000, 'ARS', 'Consultorio Test', $4, now(), now(), now())`,
				uuid.NewString(), scratchTenantA, payer, users[scratchTenantA])
			return err
		})
		expectCode(t, err, "23514", "loose charge with a client")
	})

	t.Run("a patient voucher carries its client", func(t *testing.T) {
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO vouchers (
					id, tenant_id, number, patient_id, concept, amount_minor, currency,
					practice_name, buyer_name, coverage_kind, issued_by, issued_at, created_at, updated_at
				) VALUES ($1, $2, 4, $3, 'Sesión', 150000, 'ARS', 'Consultorio Test', 'Paciente de prueba',
					'private', $4, now(), now(), now())`,
				uuid.NewString(), scratchTenantA, patients[scratchTenantA], users[scratchTenantA])
			return err
		})
		expectCode(t, err, "23514", "patient voucher without a client")
	})

	t.Run("the client must belong to the practice", func(t *testing.T) {
		// Practice B's payer on a voucher of practice A: the composite key refuses it.
		clientB := payerOf(t, scratchTenantB, patients[scratchTenantB])
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO vouchers (
					id, tenant_id, number, patient_id, client_id, concept, amount_minor, currency,
					practice_name, buyer_name, coverage_kind, issued_by, issued_at, created_at, updated_at
				) VALUES ($1, $2, 5, $3, $4, 'Sesión', 150000, 'ARS', 'Consultorio Test', 'Ajena',
					'private', $5, now(), now(), now())`,
				uuid.NewString(), scratchTenantA, patients[scratchTenantA], clientB, users[scratchTenantA])
			return err
		})
		expectCode(t, err, "23503", "foreign client")

		// The same payer belongs together with its own patient's voucher.
		if err := issueNew(t, scratchTenantB, patients[scratchTenantB], clientB, "Paciente de prueba", 2); err != nil {
			t.Fatalf("the practice's own payer was refused: %v", err)
		}
	})

	t.Run("an issued voucher cannot be re-pointed at another client", func(t *testing.T) {
		// A second payer of the practice, so the change is a real one in the same tenant.
		// Clients are under forced RLS: the insert needs the tenant set, like the app does.
		var second string
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			var inner error
			second, inner = insertClientApp(ctx, app, scratchTenantA, "Segundo pagador", "AR")
			return inner
		}); err != nil {
			t.Fatalf("setup: second payer: %v", err)
		}

		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE vouchers SET client_id = $2 WHERE tenant_id = $1 AND number = 2`, scratchTenantA, second)
			return err
		})
		expectCode(t, err, "23001", "client change on an issued voucher")

		// The buyer snapshot keeps the freeze 0020 already had.
		err = withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE vouchers SET buyer_name = 'Otro comprador' WHERE tenant_id = $1 AND number = 2`, scratchTenantA)
			return err
		})
		expectCode(t, err, "23001", "buyer change on an issued voucher")
	})

	t.Run("the down restores the 0020 shape: no column, snapshot still frozen", func(t *testing.T) {
		migrateDown(t, tempOwnerDSN)
		if got := gooseVersion(t, tempOwnerDSN); got != 26 {
			t.Fatalf("after down: goose version = %d, want 26", got)
		}

		var hasColumn bool
		if err := owner.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = 'vouchers'
				  AND column_name = 'client_id')`,
		).Scan(&hasColumn); err != nil {
			t.Fatalf("cannot inspect the vouchers table: %v", err)
		}
		if hasColumn {
			t.Fatal("the down left client_id on vouchers")
		}

		constraintExists := func(name string) bool {
			t.Helper()
			var exists bool
			if err := owner.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_constraint
					WHERE conrelid = 'vouchers'::regclass AND conname = $1
				)`, name).Scan(&exists); err != nil {
				t.Fatalf("cannot look up constraint %s: %v", name, err)
			}
			return exists
		}
		for _, name := range []string{"vouchers_client_same_tenant", "vouchers_client_of_patient", "vouchers_client_needs_patient"} {
			if constraintExists(name) {
				t.Fatalf("the down left the constraint %s", name)
			}
		}

		// The buyer snapshot is frozen the way 0020 left it, under a guard that no
		// references the dropped column.
		err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `UPDATE vouchers SET buyer_name = 'Otro comprador' WHERE tenant_id = $1 AND number = 2`, scratchTenantA)
			return err
		})
		expectCode(t, err, "23001", "buyer change after the down")

		// A void still works under the restored guard.
		if touched := voidVoucherNumber(ctx, app, scratchTenantA, 2, "Error de carga", t); touched != 1 {
			t.Fatalf("void after the down touched %d rows, want 1", touched)
		}

		// Up again: the schema that ships can be applied and is usable.
		migrateUpTo(t, tempOwnerDSN, 27)
		if got := gooseVersion(t, tempOwnerDSN); got != 27 {
			t.Fatalf("after up again: goose version = %d, want 27", got)
		}
		if err := issueNew(t, scratchTenantA, patients[scratchTenantA], payerOf(t, scratchTenantA, patients[scratchTenantA]), "Paciente de prueba", 7); err != nil {
			t.Fatalf("cannot issue under the restored schema: %v", err)
		}
	})
}

// voidVoucherNumber moves the number'th voucher of the practice to voided and answers how
// many rows the update touched.
func voidVoucherNumber(ctx context.Context, app *pgx.Conn, tenantID string, number int, reason string, t *testing.T) int64 {
	t.Helper()
	var affected int64
	err := withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
		tag, err := app.Exec(ctx, `
			UPDATE vouchers
			SET status = 'voided', voided_by = issued_by, voided_at = now(), void_reason = $2, updated_at = now()
			WHERE tenant_id = $1 AND number = $3`, tenantID, reason, number)
		affected = tag.RowsAffected()
		return err
	})
	if err != nil {
		t.Fatalf("cannot void voucher %d: %v", number, err)
	}
	return affected
}

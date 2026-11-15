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

// TestBillingMigrationsRollBack proves the Down of 0015 and 0014, in that order, undo
// what their Up did, and that a later Up restores the current schema. It runs on a
// throwaway database: the development database is not rolled back.
func TestBillingMigrationsRollBack(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the billing migration tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	if got := gooseVersion(t, tempOwnerDSN); got != 15 {
		t.Fatalf("after up: goose version = %d, want 15", got)
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

	ptr := func(s string) *string { return &s }

	expectCode := func(t *testing.T, err error, code, what string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("%s: error = %v, want SQLSTATE %s", what, err, code)
		}
	}

	otherPatient := uuid.NewString()
	if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
		_, err := app.Exec(ctx, `
			INSERT INTO patients (id, tenant_id, country, full_name, created_at, updated_at)
			VALUES ($1, $2, 'AR', 'Otra paciente', now(), now())`, otherPatient, scratchTenantA)
		return err
	}); err != nil {
		t.Fatalf("setup: second patient: %v", err)
	}

	appointment := uuid.NewString()
	start := time.Date(2026, 11, 20, 16, 0, 0, 0, time.UTC)
	if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
		_, err := app.Exec(ctx, `
			INSERT INTO appointments (
				id, tenant_id, professional_user_id, patient_id,
				starts_at, ends_at, status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, 'scheduled', now(), now())`,
			appointment, scratchTenantA, users[scratchTenantA], patients[scratchTenantA], start, start.Add(45*time.Minute))
		return err
	}); err != nil {
		t.Fatalf("setup: appointment: %v", err)
	}

	insertMismatch := func(number int) error {
		return withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO vouchers (
					id, tenant_id, number, patient_id, appointment_id, concept, amount_minor, currency,
					practice_name, buyer_name, issued_by, issued_at, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, 'Sesión', 150000, 'ARS', 'Consultorio Test', $6, $7, now(), now(), now())`,
				uuid.NewString(), scratchTenantA, number, otherPatient, appointment, ptr("Paciente"), users[scratchTenantA])
			return err
		})
	}

	indexExists := func(name string) bool {
		t.Helper()
		var exists bool
		if err := owner.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1)`, name).Scan(&exists); err != nil {
			t.Fatalf("cannot look up index %s: %v", name, err)
		}
		return exists
	}
	constraintExists := func(table, name string) bool {
		t.Helper()
		var exists bool
		if err := owner.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conrelid = $1::regclass AND conname = $2
			)`, table, name).Scan(&exists); err != nil {
			t.Fatalf("cannot look up constraint %s on %s: %v", name, table, err)
		}
		return exists
	}
	relationExists := func(name string) bool {
		t.Helper()
		var exists bool
		if err := owner.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil {
			t.Fatalf("cannot look up relation %s: %v", name, err)
		}
		return exists
	}
	functionExists := func(name string) bool {
		t.Helper()
		var exists bool
		if err := owner.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_proc WHERE proname = $1)`, name).Scan(&exists); err != nil {
			t.Fatalf("cannot look up function %s: %v", name, err)
		}
		return exists
	}

	t.Run("0015 is in place after up", func(t *testing.T) {
		if !constraintExists("vouchers", "vouchers_appointment_of_patient") {
			t.Fatal("missing vouchers_appointment_of_patient")
		}
		if !constraintExists("vouchers", "vouchers_appointment_requires_patient") {
			t.Fatal("missing vouchers_appointment_requires_patient")
		}
		if constraintExists("vouchers", "vouchers_appointment_same_tenant") {
			t.Fatal("the 0014 appointment FK should have been replaced")
		}
		if !indexExists("appointments_tenant_id_id_patient_unique") {
			t.Fatal("missing appointments_tenant_id_id_patient_unique")
		}
		expectCode(t, insertMismatch(1), "23503", "appointment of another patient under 0015")
	})

	t.Run("0015 down restores the same-tenant appointment FK", func(t *testing.T) {
		migrateDown(t, tempOwnerDSN)
		if got := gooseVersion(t, tempOwnerDSN); got != 14 {
			t.Fatalf("after 0015 down: goose version = %d, want 14", got)
		}
		if constraintExists("vouchers", "vouchers_appointment_of_patient") {
			t.Fatal("0015 down left vouchers_appointment_of_patient")
		}
		if constraintExists("vouchers", "vouchers_appointment_requires_patient") {
			t.Fatal("0015 down left vouchers_appointment_requires_patient")
		}
		if !constraintExists("vouchers", "vouchers_appointment_same_tenant") {
			t.Fatal("0015 down did not restore vouchers_appointment_same_tenant")
		}
		if indexExists("appointments_tenant_id_id_patient_unique") {
			t.Fatal("0015 down left appointments_tenant_id_id_patient_unique")
		}
		if !indexExists("appointments_tenant_id_id_unique") {
			t.Fatal("0015 down dropped the 0014 appointment index")
		}
		// The hole 0015 closed is open again: a voucher may name another patient's appointment.
		if err := insertMismatch(2); err != nil {
			t.Fatalf("after 0015 down a mismatched appointment should be accepted: %v", err)
		}
	})

	t.Run("0014 down removes the billing schema and leaves appointments", func(t *testing.T) {
		migrateDown(t, tempOwnerDSN)
		if got := gooseVersion(t, tempOwnerDSN); got != 13 {
			t.Fatalf("after 0014 down: goose version = %d, want 13", got)
		}
		if relationExists("vouchers") {
			t.Fatal("0014 down left vouchers")
		}
		if relationExists("voucher_counters") {
			t.Fatal("0014 down left voucher_counters")
		}
		if functionExists("vouchers_guard") {
			t.Fatal("0014 down left vouchers_guard")
		}
		if indexExists("appointments_tenant_id_id_unique") {
			t.Fatal("0014 down left appointments_tenant_id_id_unique")
		}
		if !relationExists("appointments") {
			t.Fatal("0014 down dropped appointments, which belong to an earlier migration")
		}
	})

	t.Run("up after the two downs restores 0015", func(t *testing.T) {
		migrate(t, tempOwnerDSN)
		if got := gooseVersion(t, tempOwnerDSN); got != 15 {
			t.Fatalf("after up: goose version = %d, want 15", got)
		}
		if !constraintExists("vouchers", "vouchers_appointment_of_patient") {
			t.Fatal("up did not restore vouchers_appointment_of_patient")
		}
		if !constraintExists("vouchers", "vouchers_appointment_requires_patient") {
			t.Fatal("up did not restore vouchers_appointment_requires_patient")
		}
		if constraintExists("vouchers", "vouchers_appointment_same_tenant") {
			t.Fatal("up left the 0014 appointment FK in place")
		}
		expectCode(t, insertMismatch(3), "23503", "appointment of another patient after up")
	})
}

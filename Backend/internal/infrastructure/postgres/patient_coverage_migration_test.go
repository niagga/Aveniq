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

// TestPatientCoverageMigrationRollsBack proves 0016 does what it says: a patient is
// private or covered by a mutual of its own practice, the billing currency is one of
// three, patients that existed before are backfilled to ARS, and its Down removes what
// its Up added. It runs on a throwaway database, never on the development one.
func TestPatientCoverageMigrationRollsBack(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the coverage migration tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)

	// The patients exist before 0016, so the backfill has something to prove.
	migrateUpTo(t, tempOwnerDSN, 15)
	seedTenants(t, ctx, tempOwnerDSN)
	patients, _ := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

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

	// Stop at 0016: 0017 is proven on its own, and this test is about 0016 alone.
	migrateUpTo(t, tempOwnerDSN, 16)
	if got := gooseVersion(t, tempOwnerDSN); got != 16 {
		t.Fatalf("after up: goose version = %d, want 16", got)
	}

	expectCode := func(t *testing.T, err error, code, what string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("%s: error = %v, want SQLSTATE %s", what, err, code)
		}
	}

	ptr := func(s string) *string { return &s }

	insertInsurer := func(tenant, name string) (string, error) {
		id := uuid.NewString()
		err := withTenantSetting(ctx, app, tenant, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO insurers (id, tenant_id, name, created_at, updated_at)
				VALUES ($1, $2, $3, now(), now())`, id, tenant, name)
			return err
		})
		return id, err
	}

	// insertPatient derives the coverage kind from the insurer, so a patient with an
	// insurer and no member number is the incoherent case the CHECK must refuse.
	insertPatient := func(tenant string, insurer, member *string, currency string) error {
		kind := "private"
		if insurer != nil {
			kind = "insurer"
		}
		return withTenantSetting(ctx, app, tenant, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO patients (
					id, tenant_id, country, full_name,
					coverage_kind, insurer_id, member_number, billing_currency,
					created_at, updated_at
				) VALUES ($1, $2, 'AR', 'Paciente', $3, $4, $5, $6, now(), now())`,
				uuid.NewString(), tenant, kind, insurer, member, currency)
			return err
		})
	}

	columnExists := func(table, column string) bool {
		t.Helper()
		var exists bool
		if err := owner.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = $1 AND column_name = $2
			)`, table, column).Scan(&exists); err != nil {
			t.Fatalf("cannot look up column %s.%s: %v", table, column, err)
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

	t.Run("existing patients are private and billed in ARS", func(t *testing.T) {
		var kind, currency string
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			return app.QueryRow(ctx, `
				SELECT coverage_kind, billing_currency FROM patients WHERE id = $1`,
				patients[scratchTenantA]).Scan(&kind, &currency)
		}); err != nil {
			t.Fatalf("cannot read the seeded patient: %v", err)
		}
		if kind != "private" || currency != "ARS" {
			t.Fatalf("seeded patient = (%s, %s), want (private, ARS)", kind, currency)
		}
	})

	insurerA, err := insertInsurer(scratchTenantA, "OSDE")
	if err != nil {
		t.Fatalf("setup: insurer: %v", err)
	}

	t.Run("an insured patient needs an insurer of its practice and a member number", func(t *testing.T) {
		if err := insertPatient(scratchTenantA, ptr(insurerA), ptr("12345"), "ARS"); err != nil {
			t.Fatalf("insured patient with insurer and member: %v", err)
		}
		if err := insertPatient(scratchTenantA, ptr(insurerA), nil, "ARS"); err == nil {
			t.Fatal("insured patient without member number was accepted")
		} else {
			expectCode(t, err, "23514", "insured patient without member number")
		}
		expectCode(t, insertPatient(scratchTenantA, ptr(insurerA), ptr("   "), "ARS"),
			"23514", "blank member number")
		expectCode(t, insertPatient(scratchTenantB, ptr(insurerA), ptr("12345"), "ARS"),
			"23503", "insurer of another practice")
	})

	t.Run("the billing currency is one of ARS, EUR or USD", func(t *testing.T) {
		if err := insertPatient(scratchTenantA, nil, nil, "EUR"); err != nil {
			t.Fatalf("private patient billed in EUR: %v", err)
		}
		if err := insertPatient(scratchTenantA, nil, nil, "USD"); err != nil {
			t.Fatalf("private patient billed in USD: %v", err)
		}
		expectCode(t, insertPatient(scratchTenantA, nil, nil, "BRL"), "23514", "unsupported currency")
		expectCode(t, insertPatient(scratchTenantA, nil, nil, "ars"), "23514", "lowercase currency")
	})

	t.Run("a mutual name is unique per practice, whatever the casing", func(t *testing.T) {
		expectCode(t, func() error { _, err := insertInsurer(scratchTenantA, "  osde "); return err }(),
			"23505", "same mutual, other casing and spaces")
		if _, err := insertInsurer(scratchTenantB, "OSDE"); err != nil {
			t.Fatalf("the same mutual name in another practice should be allowed: %v", err)
		}
	})

	t.Run("a practice cannot write or see another practice's mutuals", func(t *testing.T) {
		expectCode(t, withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO insurers (id, tenant_id, name, created_at, updated_at)
				VALUES ($1, $2, 'Intrusa', now(), now())`, uuid.NewString(), scratchTenantA)
			return err
		}), rlsViolationCode, "insert into another practice")

		var visible int
		if err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			return app.QueryRow(ctx, `SELECT count(*) FROM insurers WHERE id = $1`, insurerA).Scan(&visible)
		}); err != nil {
			t.Fatalf("cannot count mutuals as the other practice: %v", err)
		}
		if visible != 0 {
			t.Fatalf("the other practice sees %d of this practice's mutual(s)", visible)
		}
	})

	t.Run("0016 down removes the coverage model", func(t *testing.T) {
		migrateDown(t, tempOwnerDSN)
		if got := gooseVersion(t, tempOwnerDSN); got != 15 {
			t.Fatalf("after 0016 down: goose version = %d, want 15", got)
		}
		for _, column := range []string{"coverage_kind", "insurer_id", "member_number", "billing_currency"} {
			if columnExists("patients", column) {
				t.Fatalf("0016 down left patients.%s", column)
			}
		}
		if relationExists("insurers") {
			t.Fatal("0016 down left insurers")
		}
		if !relationExists("patients") {
			t.Fatal("0016 down dropped patients, which belong to an earlier migration")
		}
	})

	t.Run("up after the down restores the coverage model", func(t *testing.T) {
		migrateUpTo(t, tempOwnerDSN, 16)
		if got := gooseVersion(t, tempOwnerDSN); got != 16 {
			t.Fatalf("after up: goose version = %d, want 16", got)
		}
		if !relationExists("insurers") {
			t.Fatal("up did not restore insurers")
		}
		for _, column := range []string{"coverage_kind", "insurer_id", "member_number", "billing_currency"} {
			if !columnExists("patients", column) {
				t.Fatalf("up did not restore patients.%s", column)
			}
		}
	})
}

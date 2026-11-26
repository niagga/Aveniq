package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// migration 28 removes the fiscal mirror from patients (T85).
const mirrorUpTo int64 = 28

// The five columns and the constraints that existed only for them. Postgres
// drops the constraints with the columns; the down restores both, never the data.
var (
	mirrorColumns = []string{"tax_id", "tax_id_kind", "fiscal_condition", "fiscal_address", "billing_currency"}
	mirrorConstraints = []string{
		"patients_tax_id_complete",
		"patients_fiscal_condition_country",
		"patients_tax_id_kind_fk",
		"patients_fiscal_condition_fk",
		"patients_billing_currency_known",
	}
)

// hasColumn reports whether the table carries the column in the current schema.
func hasColumn(t *testing.T, conn *pgx.Conn, table, column string) bool {
	t.Helper()
	var present bool
	if err := conn.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2
		)`, table, column).Scan(&present); err != nil {
		t.Fatalf("cannot inspect %s.%s: %v", table, column, err)
	}
	return present
}

// hasConstraint reports whether the named constraint exists in the database.
func hasConstraint(t *testing.T, conn *pgx.Conn, name string) bool {
	t.Helper()
	var known bool
	if err := conn.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = $1)`, name).Scan(&known); err != nil {
		t.Fatalf("cannot look up constraint %s: %v", name, err)
	}
	return known
}

// insertClientWithFiscalData seeds a billing client carrying fiscal data, under
// the open tenant scope, and returns its id.
func insertClientWithFiscalData(ctx context.Context, conn *pgx.Conn, tenantID string) (string, error) {
	clientID := uuid.NewString()
	_, err := conn.Exec(ctx, `
		INSERT INTO clients (
			id, tenant_id, display_name, country,
			tax_id, tax_id_kind, fiscal_condition, fiscal_address, billing_currency,
			is_active, created_at, updated_at
		) VALUES ($1, $2, 'Paciente de prueba', 'AR', '20123456786', 'cuit', 'monotributo', 'Av. Siempre Viva 742', 'ARS', true, now(), now())`,
		clientID, tenantID)
	return clientID, err
}

// insertPatientWithClient creates the billing client and its patient the way the
// application writes them from 0028 on: the patient row carries no fiscal data,
// and the client keeps carrying it.
func insertPatientWithClient(t *testing.T, ctx context.Context, app *pgx.Conn, tenantID string) error {
	t.Helper()
	return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
		clientID, err := insertClientWithFiscalData(ctx, app, tenantID)
		if err != nil {
			return err
		}
		_, err = app.Exec(ctx, `
			INSERT INTO patients (id, tenant_id, client_id, country, full_name, created_at, updated_at)
			VALUES ($1, $2, $3, 'AR', 'Paciente de prueba', now(), now())`,
			uuid.NewString(), tenantID, clientID)
		return err
	})
}

// TestPatientFiscalMirrorMigration pins what 0028 does: the fiscal mirror and the
// billing currency leave the patients row (T85). Since T72 the data lives on the
// billing client, and since T81 nothing reads the copy, so the migration drops the
// five columns together with the constraints that only existed for them. It also
// proves a patient is still created together with its billing client, which keeps
// carrying the data. The Down restores the shape the mirror had, not the data:
// the migration removes a copy that nothing read, and the owner authorized the
// removal of that data. It runs on a throwaway database, never on the
// development one.
func TestPatientFiscalMirrorMigration(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the migration tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	if got := gooseVersion(t, tempOwnerDSN); got != mirrorUpTo {
		t.Fatalf("after up: goose version = %d, want %d", got, mirrorUpTo)
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

	t.Run("0028 up drops the mirror from patients and keeps it on the client", func(t *testing.T) {
		for _, column := range mirrorColumns {
			if hasColumn(t, owner, "patients", column) {
				t.Fatalf("patients.%s must be gone after 0028", column)
			}
		}
		for _, column := range mirrorColumns {
			if !hasColumn(t, owner, "clients", column) {
				t.Fatalf("clients.%s must stay: the fiscal data belongs to the billing client", column)
			}
		}
	})

	t.Run("a patient is still created with its client, which carries the data", func(t *testing.T) {
		if err := insertPatientWithClient(t, ctx, app, scratchTenantA); err != nil {
			t.Fatalf("cannot create a patient with its billing client after 0028: %v", err)
		}

		var taxID, taxKind, fiscalCondition, fiscalAddress, currency string
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			return app.QueryRow(ctx, `
				SELECT tax_id, tax_id_kind, fiscal_condition, fiscal_address, billing_currency
				FROM clients WHERE display_name = 'Paciente de prueba' AND country = 'AR'
				AND tax_id IS NOT NULL LIMIT 1`).Scan(&taxID, &taxKind, &fiscalCondition, &fiscalAddress, &currency)
		}); err != nil {
			t.Fatalf("cannot read the client's fiscal data: %v", err)
		}
		if taxID != "20123456786" || taxKind != "cuit" || fiscalCondition != "monotributo" || fiscalAddress != "Av. Siempre Viva 742" || currency != "ARS" {
			t.Fatalf("client = (%q, %q, %q, %q, %q), want the fiscal data on the client row", taxID, taxKind, fiscalCondition, fiscalAddress, currency)
		}
	})

	t.Run("0028 down restores the shape of the mirror, not its data", func(t *testing.T) {
		migrateDown(t, tempOwnerDSN)
		if got := gooseVersion(t, tempOwnerDSN); got != mirrorUpTo-1 {
			t.Fatalf("after down: goose version = %d, want %d", got, mirrorUpTo-1)
		}
		for _, column := range mirrorColumns {
			if !hasColumn(t, owner, "patients", column) {
				t.Fatalf("the down must restore patients.%s", column)
			}
		}
		for _, constraint := range mirrorConstraints {
			if !hasConstraint(t, owner, constraint) {
				t.Fatalf("the down must restore the constraint %s", constraint)
			}
		}

		var nullable, hasDefault string
		if err := owner.QueryRow(ctx, `
			SELECT is_nullable, coalesce(column_default, '')
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'patients' AND column_name = 'billing_currency'`).Scan(&nullable, &hasDefault); err != nil {
			t.Fatalf("cannot read the restored billing_currency shape: %v", err)
		}
		if nullable != "NO" || hasDefault == "" {
			t.Fatalf("billing_currency restored as (nullable %q, default %q), want NOT NULL with the default it was born with", nullable, hasDefault)
		}
	})

	t.Run("up again: the mirror is gone once more", func(t *testing.T) {
		// Goose Up beyond the down proves the down left a schema 0028 can re-apply to.
		migrateUpTo(t, tempOwnerDSN, mirrorUpTo)
		for _, column := range mirrorColumns {
			if hasColumn(t, owner, "patients", column) {
				t.Fatalf("patients.%s must be gone after re-applying 0028", column)
			}
		}
	})
}

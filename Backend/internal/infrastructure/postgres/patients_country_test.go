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

// TestPatientsCountryModel pins the property 0005 exists for: a document kind is
// valid only inside its own country, and the database is the one saying so.
//
// The alternative — a CHECK with Argentine codes — could only ever answer "this
// kind exists", never "this kind exists here". These tests would all pass with the
// wrong country and the wrong kind.
func TestPatientsCountryModel(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the schema tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	app, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = app.Close(ctx) }()

	type patient struct {
		country      *string
		documentType *string
		document     *string
		normalized   *string
		taxKind      *string
		fiscalCond   *string
	}

	insert := func(ctx context.Context, tenantID string, row patient) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO patients (
					id, tenant_id, full_name, country, document_type, document_number,
					document_normalized, tax_id_kind, fiscal_condition, created_at, updated_at
				) VALUES ($1, $2, 'Paciente de prueba', $3, $4, $5, $6, $7, $8, now(), now())`,
				uuid.NewString(), tenantID, row.country, row.documentType, row.document,
				row.normalized, row.taxKind, row.fiscalCond)
			return err
		})
	}

	text := func(value string) *string { return &value }

	expectCode := func(t *testing.T, err error, code, what string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("%s: error = %v, want SQLSTATE %s", what, err, code)
		}
	}

	t.Run("tenants pick a country and default to Argentina", func(t *testing.T) {
		var country string
		if err := app.QueryRow(ctx, "SELECT country FROM tenants LIMIT 1").Scan(&country); err != nil {
			t.Fatalf("cannot read the tenant country: %v", err)
		}
		if country != "AR" {
			t.Fatalf("country = %q, want the AR default", country)
		}
	})

	t.Run("a Spanish NIE is accepted where an Argentine DNI would not be", func(t *testing.T) {
		nie := patient{
			country:      text("ES"),
			documentType: text("nie"),
			document:     text("X1234567L"),
			normalized:   text("X1234567L"),
		}
		if err := insert(ctx, scratchTenantA, nie); err != nil {
			t.Fatalf("a valid NIE must be accepted, alphanumeric and all: %v", err)
		}

		// The same kind under the wrong country: this is the case a CHECK of codes
		// could never refuse.
		wrongCountry := patient{
			country:      text("AR"),
			documentType: text("nie"),
			document:     text("X1234567L"),
			normalized:   text("X1234567L"),
		}
		expectCode(t, insert(ctx, scratchTenantB, wrongCountry), "23503", "NIE under AR")
	})

	t.Run("a document without its country is refused", func(t *testing.T) {
		half := patient{documentType: text("dni"), document: text("30111222"), normalized: text("30111222")}
		expectCode(t, insert(ctx, scratchTenantB, half), "23514", "document without country")
	})

	t.Run("fiscal condition belongs to the country too", func(t *testing.T) {
		inArgentina := patient{country: text("AR"), fiscalCond: text("monotributo")}
		if err := insert(ctx, scratchTenantA, inArgentina); err != nil {
			t.Fatalf("monotributo is a valid AR condition: %v", err)
		}

		inPortugal := patient{country: text("PT"), fiscalCond: text("monotributo")}
		expectCode(t, insert(ctx, scratchTenantB, inPortugal), "23503", "monotributo under PT")
	})

	t.Run("the unique index compares the normalized document", func(t *testing.T) {
		first := patient{
			country:      text("AR"),
			documentType: text("dni"),
			document:     text("30.111.222"),
			normalized:   text("30111222"),
		}
		if err := insert(ctx, scratchTenantA, first); err != nil {
			t.Fatalf("cannot insert the first patient: %v", err)
		}

		// Different as typed, identical once normalized: the same person.
		second := patient{
			country:      text("AR"),
			documentType: text("dni"),
			document:     text("30111222"),
			normalized:   text("30111222"),
		}
		expectCode(t, insert(ctx, scratchTenantA, second), "23505", "same document, other separators")
	})

	t.Run("the catalogs are readable but not writable by the application role", func(t *testing.T) {
		var total int
		if err := app.QueryRow(ctx, "SELECT count(*) FROM document_kinds").Scan(&total); err != nil {
			t.Fatalf("the application role must be able to read the catalogs: %v", err)
		}
		if total == 0 {
			t.Fatal("the document catalog is empty: the seed did not run")
		}

		// Adding a country is a migration, not a request. Without this revoke, the
		// application could invent catalog rows at runtime.
		_, err := app.Exec(ctx, "INSERT INTO countries (code, name) VALUES ('XX', 'Inventado')")
		expectCode(t, err, "42501", "writing to the country catalog")
	})
}

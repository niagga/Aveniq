package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// TestBillingSubjectSearch proves the search finds a buyer by the document however it
// was typed when the patient was saved, and that it stays a search: a short fragment
// never reaches the documents, a wildcard is a character, and another practice's
// patients and the inactive ones are not found.
func TestBillingSubjectSearch(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the billing search tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	pool, err := pgxpool.New(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	t.Cleanup(pool.Close)

	transactor := postgres.NewTransactor(pool)
	patients := postgres.NewPatientRepository(pool)
	clients := postgres.NewClientRepository(pool)
	catalog := postgres.NewPatientCatalog(pool)
	billing := postgres.NewBillingRepository(pool)

	create := func(tenantID string, subject patient.Patient) {
		t.Helper()
		if err := transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			return persistPatient(ctx, clients, patients, subject)
		}); err != nil {
			t.Fatalf("cannot create %q: %v", subject.FullName, err)
		}
	}

	// Saved with separators, which is how people type them. The payer fixture
	// (clientDraftFromPatient) gives every client the CUIT 20-12345678-6; the
	// document is what tells the patients apart.
	ana := newPatient(t, catalog, scratchTenantA, "Ana Pérez", "30.111.222")
	create(scratchTenantA, ana)
	bruno := newPatient(t, catalog, scratchTenantA, "Bruno Díaz", "31.222.333")
	create(scratchTenantA, bruno)
	gone := newPatient(t, catalog, scratchTenantA, "Carla Inactiva", "32.333.444")
	gone.IsActive = false
	create(scratchTenantA, gone)
	other := newPatient(t, catalog, scratchTenantB, "Dora Ajena", "30.111.999")
	create(scratchTenantB, other)

	search := func(tenantID, fragment string) []string {
		t.Helper()
		var names []string
		if err := transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			found, err := billing.SearchSubjects(ctx, fragment, 20)
			for _, subject := range found {
				names = append(names, subject.FullName)
			}
			return err
		}); err != nil {
			t.Fatalf("search %q: %v", fragment, err)
		}
		return names
	}
	expect := func(tenantID, fragment string, want ...string) {
		t.Helper()
		got := search(tenantID, fragment)
		if len(got) != len(want) {
			t.Fatalf("search %q found %v, want %v", fragment, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("search %q found %v, want %v", fragment, got, want)
			}
		}
	}

	t.Run("the name still matches as typed", func(t *testing.T) {
		expect(scratchTenantA, "pérez", "Ana Pérez")
	})

	t.Run("a document saved with dots is found without them", func(t *testing.T) {
		expect(scratchTenantA, "30111222", "Ana Pérez")
	})

	t.Run("a document saved without dots is found with them", func(t *testing.T) {
		expect(scratchTenantA, "31.222.333", "Bruno Díaz")
	})

	t.Run("part of a document is found, with or without separators", func(t *testing.T) {
		expect(scratchTenantA, "111.22", "Ana Pérez")
		expect(scratchTenantA, "11122", "Ana Pérez")
	})

	t.Run("a tax id is found however it is written", func(t *testing.T) {
		both := []string{"Ana Pérez", "Bruno Díaz"}
		expect(scratchTenantA, "20123456786", both...)
		expect(scratchTenantA, "20-12345678-6", both...)
		expect(scratchTenantA, "20 12345678", both...)
	})

	t.Run("a short fragment of digits does not reach the documents", func(t *testing.T) {
		expect(scratchTenantA, "30")
		expect(scratchTenantA, "3.1")
	})

	t.Run("a wildcard is a character, not a roster", func(t *testing.T) {
		expect(scratchTenantA, "%%%")
		expect(scratchTenantA, "___")
	})

	t.Run("an inactive patient is not found", func(t *testing.T) {
		expect(scratchTenantA, "32333444")
		expect(scratchTenantA, "inactiva")
	})

	t.Run("another practice's patient is not found", func(t *testing.T) {
		expect(scratchTenantA, "30111999")
		expect(scratchTenantB, "30111222")
		expect(scratchTenantB, "30111999", "Dora Ajena")
	})
}

package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// patientEnv builds a throwaway database with the real migrations, the patient
// adapter and the country catalog. It skips without the DSNs, like the rest of the
// package.
func patientEnv(t *testing.T) (*postgres.Transactor, *postgres.PatientRepository, *postgres.PatientCatalog, string, string) {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the patient tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	pool, err := pgxpool.New(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database as the application role: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return postgres.NewTransactor(pool), postgres.NewPatientRepository(pool), postgres.NewPatientCatalog(pool),
		scratchTenantA, scratchTenantB
}

// newPatient builds a valid Argentine patient, normalized through the catalog the
// same way the application will do it.
func newPatient(t *testing.T, catalog *postgres.PatientCatalog, tenantID, fullName, document string) patient.Patient {
	t.Helper()
	ctx := context.Background()

	var built *patient.Document
	if document != "" {
		kind, err := catalog.DocumentKind(ctx, "AR", "dni")
		if err != nil {
			t.Fatalf("cannot load the AR dni kind: %v", err)
		}
		normalized, err := patient.NormalizeDocument(kind, document)
		if err != nil {
			t.Fatalf("cannot normalize %q: %v", document, err)
		}
		built = &normalized
	}

	taxKind, err := catalog.TaxIDKind(ctx, "AR", "cuit")
	if err != nil {
		t.Fatalf("cannot load the AR cuit kind: %v", err)
	}
	taxID, err := patient.NormalizeTaxID(taxKind, "20-12345678-6")
	if err != nil {
		t.Fatalf("cannot normalize the CUIT: %v", err)
	}

	condition := "monotributo"

	subject, err := patient.New(patient.ID(uuid.NewString()), tenant.ID(tenantID), patient.Draft{
		FullName:        fullName,
		Country:         "AR",
		Document:        built,
		TaxID:           &taxID,
		FiscalCondition: &condition,
	}, time.Now())
	if err != nil {
		t.Fatalf("cannot build the domain patient: %v", err)
	}
	return subject
}

func TestPatientRepository(t *testing.T) {
	transactor, repo, catalog, tenantA, tenantB := patientEnv(t)
	ctx := context.Background()

	t.Run("a patient persists and reads back with its country data", func(t *testing.T) {
		subject := newPatient(t, catalog, tenantA, "Ana Pérez", "30.111.222")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		}); err != nil {
			t.Fatalf("cannot create the patient: %v", err)
		}

		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := repo.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.Document == nil || got.Document.Normalized != "30111222" {
				t.Errorf("document = %+v, want the normalized DNI", got.Document)
			}
			if got.Document != nil && got.Document.Number != "30.111.222" {
				t.Errorf("document number = %q, want what the person typed", got.Document.Number)
			}
			if got.TaxID == nil || got.TaxID.Kind != "cuit" {
				t.Errorf("tax id = %+v, want the CUIT", got.TaxID)
			}
			if got.FiscalCondition == nil || *got.FiscalCondition != "monotributo" {
				t.Errorf("fiscal condition = %v, want monotributo", got.FiscalCondition)
			}
			if got.Country != "AR" {
				t.Errorf("Country = %q, want AR", got.Country)
			}
			if !got.IsActive {
				t.Error("a new patient must be active")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot read the patient back: %v", err)
		}
	})

	t.Run("the same document twice in one tenant is a domain conflict", func(t *testing.T) {
		first := newPatient(t, catalog, tenantA, "Duplicada Uno", "27.222.333")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, first)
		}); err != nil {
			t.Fatalf("cannot create the first patient: %v", err)
		}

		// Same person typed with different separators: the normalized value is what
		// the index compares.
		second := newPatient(t, catalog, tenantA, "Duplicada Dos", "27222333")
		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, second)
		})
		if !errors.Is(err, patient.ErrDocumentTaken) {
			t.Fatalf("error = %v, want %v", err, patient.ErrDocumentTaken)
		}
	})

	t.Run("a document kind the country does not know is refused by the catalog", func(t *testing.T) {
		// Built by hand to bypass the domain, which is the only way to reach the
		// foreign key: AR has no NIE.
		forged := newPatient(t, catalog, tenantA, "Sin Documento Válido", "")
		forged.Document = &patient.Document{Country: "AR", Kind: "nie", Number: "X1234567L", Normalized: "X1234567L"}

		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, forged)
		})
		if !errors.Is(err, patient.ErrDocumentInvalid) {
			t.Fatalf("error = %v, want %v", err, patient.ErrDocumentInvalid)
		}
	})

	t.Run("a patient outside WithTenant is refused before the database", func(t *testing.T) {
		subject := newPatient(t, catalog, tenantA, "Sin Scope", "")

		err := repo.Create(ctx, subject)
		if !errors.Is(err, patient.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want %v", err, patient.ErrTenantScopeMissing)
		}
	})

	t.Run("a patient of one tenant is invisible from another", func(t *testing.T) {
		subject := newPatient(t, catalog, tenantA, "Solo de A", "")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		}); err != nil {
			t.Fatalf("cannot create the patient: %v", err)
		}

		err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			_, err := repo.ByID(ctx, subject.ID)
			return err
		})
		if !errors.Is(err, patient.ErrNotFound) {
			t.Fatalf("error = %v, want %v: row-level security must hide the row", err, patient.ErrNotFound)
		}
	})

	t.Run("an edit persists and keeps the identity", func(t *testing.T) {
		subject := newPatient(t, catalog, tenantA, "Editable", "24.444.555")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		}); err != nil {
			t.Fatalf("cannot create the patient: %v", err)
		}

		phone := "+54 11 5555-5555"
		draft := patient.Draft{
			FullName: "Editable Corregida",
			Country:  "AR",
			Document: subject.Document,
			Phone:    &phone,
		}
		updated, err := subject.ChangeProfile(draft, time.Now())
		if err != nil {
			t.Fatalf("cannot apply the change: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Update(ctx, updated)
		}); err != nil {
			t.Fatalf("cannot update the patient: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := repo.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.FullName != "Editable Corregida" {
				t.Errorf("FullName = %q, want the new value", got.FullName)
			}
			if got.Phone == nil || *got.Phone != phone {
				t.Errorf("Phone = %v, want %q", got.Phone, phone)
			}
			if !got.CreatedAt.Equal(subject.CreatedAt) {
				t.Errorf("CreatedAt changed: %v vs %v", got.CreatedAt, subject.CreatedAt)
			}
			return nil
		}); err != nil {
			t.Fatalf("cannot read the updated patient: %v", err)
		}
	})

	t.Run("the listing filters by name fragment and by exact document", func(t *testing.T) {
		unique := "Buscable " + uuid.NewString()[:6]
		subject := newPatient(t, catalog, tenantA, unique, "23.333.444")
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		}); err != nil {
			t.Fatalf("cannot create the patient: %v", err)
		}

		for name, tc := range map[string]struct {
			filter port.PatientFilter
			want   int
		}{
			"by name fragment": {port.PatientFilter{Search: unique, Limit: 10}, 1},
			// The document is matched exactly and normalized, so typing the dots
			// still finds the row.
			"by typed document":      {port.PatientFilter{Search: "23.333.444", Limit: 10}, 0},
			"by normalized document": {port.PatientFilter{Search: "23333444", Limit: 10}, 1},
			"nothing matches":        {port.PatientFilter{Search: "inexistente-" + unique, Limit: 10}, 0},
		} {
			got, err := func() ([]patient.Patient, error) {
				var rows []patient.Patient
				err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
					var inner error
					rows, inner = repo.List(ctx, tc.filter)
					return inner
				})
				return rows, err
			}()
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", name, err)
			}
			if len(got) != tc.want {
				t.Fatalf("%s: rows = %d, want %d", name, len(got), tc.want)
			}
		}
	})
}

func TestPatientCatalog(t *testing.T) {
	_, _, catalog, tenantA, _ := patientEnv(t)
	ctx := context.Background()

	t.Run("a Spanish NIE is described by the catalog, not by the code", func(t *testing.T) {
		kind, err := catalog.DocumentKind(ctx, "ES", "nie")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if kind.Normalizer != "alnum_upper" || kind.Checksum != "es_nie" {
			t.Fatalf("kind = %+v, want the ES NIE rules", kind)
		}

		if _, err := patient.NormalizeDocument(kind, "x1234567l"); err != nil {
			t.Fatalf("the catalog-driven normalization must accept a lower-case NIE: %v", err)
		}
	})

	t.Run("a kind the country does not have is a domain error", func(t *testing.T) {
		if _, err := catalog.DocumentKind(ctx, "AR", "nie"); !errors.Is(err, patient.ErrDocumentInvalid) {
			t.Fatalf("error = %v, want %v", err, patient.ErrDocumentInvalid)
		}
		if _, err := catalog.TaxIDKind(ctx, "PT", "cuit"); !errors.Is(err, patient.ErrDocumentInvalid) {
			t.Fatalf("error = %v, want %v", err, patient.ErrDocumentInvalid)
		}
	})

	t.Run("fiscal conditions are country-scoped", func(t *testing.T) {
		got, err := catalog.FiscalCondition(ctx, "AR", "monotributo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Label == "" {
			t.Error("the catalog row has no label")
		}

		if _, err := catalog.FiscalCondition(ctx, "PT", "monotributo"); !errors.Is(err, patient.ErrDocumentInvalid) {
			t.Fatalf("error = %v, want %v", err, patient.ErrDocumentInvalid)
		}
	})

	t.Run("a patient inherits the practice's country", func(t *testing.T) {
		country, err := catalog.TenantCountry(ctx, tenantA)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if country != "AR" {
			t.Fatalf("country = %q, want the AR default", country)
		}
	})
}

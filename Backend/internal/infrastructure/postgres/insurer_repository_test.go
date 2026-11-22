package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// insurerEnv builds a throwaway database with the real migrations and the insurer
// and patient adapters, so the patient-to-mutual foreign key is exercised for real.
func insurerEnv(t *testing.T) (*postgres.Transactor, *postgres.InsurerRepository, *postgres.PatientRepository, *postgres.ClientRepository, string, string) {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the insurer tests")
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

	return postgres.NewTransactor(pool),
		postgres.NewInsurerRepository(pool),
		postgres.NewPatientRepository(pool),
		postgres.NewClientRepository(pool),
		scratchTenantA, scratchTenantB
}

// newInsurerFor builds a valid mutual of the practice without persisting it.
func newInsurerFor(t *testing.T, tenantID, name string) insurer.Insurer {
	t.Helper()
	subject, err := insurer.New(insurer.ID(uuid.NewString()), tenant.ID(tenantID), insurer.Draft{Name: name}, time.Now())
	if err != nil {
		t.Fatalf("cannot build the mutual %q: %v", name, err)
	}
	return subject
}

func TestInsurerRepository(t *testing.T) {
	transactor, repo, _, _, tenantA, tenantB := insurerEnv(t)
	ctx := context.Background()

	create := func(t *testing.T, tenantID string, subject insurer.Insurer) error {
		t.Helper()
		return transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			return repo.Create(ctx, subject)
		})
	}

	t.Run("a mutual persists and reads back with its tax id", func(t *testing.T) {
		subject, err := insurer.New(insurer.ID(uuid.NewString()), tenant.ID(tenantA),
			insurer.Draft{Name: "OSDE", TaxID: strPtr("30-12345678-9")}, time.Now())
		if err != nil {
			t.Fatalf("cannot build the mutual: %v", err)
		}
		if err := create(t, tenantA, subject); err != nil {
			t.Fatalf("cannot create the mutual: %v", err)
		}

		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := repo.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.Name != "OSDE" {
				t.Errorf("Name = %q, want OSDE", got.Name)
			}
			if got.TaxID == nil || *got.TaxID != "30123456789" {
				t.Errorf("TaxID = %v, want the normalized 11 digits", got.TaxID)
			}
			if !got.IsActive {
				t.Error("a new mutual must be active")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot read the mutual back: %v", err)
		}
	})

	t.Run("the same name in one practice is a domain conflict, whatever the case and spaces", func(t *testing.T) {
		if err := create(t, tenantA, newInsurerFor(t, tenantA, "Galeno")); err != nil {
			t.Fatalf("cannot create the first mutual: %v", err)
		}

		err := create(t, tenantA, newInsurerFor(t, tenantA, "  galeno "))
		if !errors.Is(err, insurer.ErrNameTaken) {
			t.Fatalf("error = %v, want %v", err, insurer.ErrNameTaken)
		}
	})

	t.Run("the same name in another practice is allowed", func(t *testing.T) {
		if err := create(t, tenantA, newInsurerFor(t, tenantA, "Swiss Medical")); err != nil {
			t.Fatalf("cannot create the mutual in A: %v", err)
		}
		if err := create(t, tenantB, newInsurerFor(t, tenantB, "Swiss Medical")); err != nil {
			t.Fatalf("the same mutual name in another practice must be allowed: %v", err)
		}
	})

	t.Run("a mutual of one practice is invisible from another", func(t *testing.T) {
		subject := newInsurerFor(t, tenantA, "Solo de A Mutual")
		if err := create(t, tenantA, subject); err != nil {
			t.Fatalf("cannot create the mutual: %v", err)
		}

		err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			_, err := repo.ByID(ctx, subject.ID)
			return err
		})
		if !errors.Is(err, insurer.ErrNotFound) {
			t.Fatalf("error = %v, want %v: row-level security must hide the row", err, insurer.ErrNotFound)
		}
	})

	t.Run("an edit deactivates the mutual and the active listing hides it", func(t *testing.T) {
		subject := newInsurerFor(t, tenantA, "Medifé")
		if err := create(t, tenantA, subject); err != nil {
			t.Fatalf("cannot create the mutual: %v", err)
		}

		inactive, err := subject.Deactivate(time.Now())
		if err != nil {
			t.Fatalf("cannot deactivate: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.Update(ctx, inactive)
		}); err != nil {
			t.Fatalf("cannot update the mutual: %v", err)
		}

		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			active, err := repo.List(ctx, true)
			if err != nil {
				return err
			}
			for _, row := range active {
				if row.ID == subject.ID {
					t.Error("the active listing must hide a deactivated mutual")
				}
			}

			all, err := repo.List(ctx, false)
			if err != nil {
				return err
			}
			found := false
			for _, row := range all {
				if row.ID == subject.ID {
					found = true
					if row.IsActive {
						t.Error("the full listing must show the mutual as inactive")
					}
				}
			}
			if !found {
				t.Error("the full listing must include the deactivated mutual")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot list the mutuals: %v", err)
		}
	})

	t.Run("a mutual outside WithTenant is refused before the database", func(t *testing.T) {
		err := repo.Create(ctx, newInsurerFor(t, tenantA, "Sin Scope Mutual"))
		if !errors.Is(err, insurer.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want %v", err, insurer.ErrTenantScopeMissing)
		}
	})
}

func TestPatientCoverageAgainstMutuals(t *testing.T) {
	transactor, insurers, patients, clients, tenantA, tenantB := insurerEnv(t)
	ctx := context.Background()

	mutualA := newInsurerFor(t, tenantA, "OSDE Coverage")
	if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		return insurers.Create(ctx, mutualA)
	}); err != nil {
		t.Fatalf("cannot create the mutual of A: %v", err)
	}

	insuredIn := func(tenantID string, insurerID string) patient.Patient {
		t.Helper()
		subject, err := patient.New(patient.ID(uuid.NewString()), tenant.ID(tenantID), patient.Draft{
			FullName:        "Paciente con cobertura",
			Country:         "AR",
			CoverageKind:    patient.CoverageInsurer,
			InsurerID:       strPtr(insurerID),
			MemberNumber:    strPtr("A-12345"),
			BillingCurrency: patient.CurrencyEUR,
		}, time.Now())
		if err != nil {
			t.Fatalf("cannot build the insured patient: %v", err)
		}
		return subject
	}

	t.Run("an insured patient persists with its mutual, member number and currency", func(t *testing.T) {
		subject := insuredIn(tenantA, string(mutualA.ID))
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return persistPatient(ctx, clients, patients, subject)
		}); err != nil {
			t.Fatalf("cannot create the insured patient: %v", err)
		}

		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := patients.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.CoverageKind != patient.CoverageInsurer {
				t.Errorf("CoverageKind = %q, want insurer", got.CoverageKind)
			}
			if got.InsurerID == nil || *got.InsurerID != string(mutualA.ID) {
				t.Errorf("InsurerID = %v, want the mutual of A", got.InsurerID)
			}
			if got.MemberNumber == nil || *got.MemberNumber != "A-12345" {
				t.Errorf("MemberNumber = %v, want A-12345", got.MemberNumber)
			}
			if got.BillingCurrency != patient.CurrencyEUR {
				t.Errorf("BillingCurrency = %q, want EUR", got.BillingCurrency)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot read the insured patient back: %v", err)
		}
	})

	t.Run("a patient cannot be covered by another practice's mutual", func(t *testing.T) {
		subject := insuredIn(tenantB, string(mutualA.ID))
		err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			return persistPatient(ctx, clients, patients, subject)
		})

		var validation *patient.ValidationError
		if !errors.As(err, &validation) || validation.Field != "insurer_id" {
			t.Fatalf("error = %v, want a ValidationError on insurer_id", err)
		}
	})
}

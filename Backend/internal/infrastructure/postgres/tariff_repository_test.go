package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// tariffEnv builds a throwaway database with the real migrations, the tariff and insurer
// adapters, and one professional per practice, so the composite foreign keys are exercised
// for real. appDSN is the application role, for the statements the adapter does not issue.
type tariffEnv struct {
	transactor    *postgres.Transactor
	tariffs       *postgres.TariffRepository
	insurers      *postgres.InsurerRepository
	professionals map[string]string
	tenantA       string
	tenantB       string
	appDSN        string
}

func newTariffEnv(t *testing.T) tariffEnv {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the tariff tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)
	_, users := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

	pool, err := pgxpool.New(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database as the application role: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return tariffEnv{
		transactor:    postgres.NewTransactor(pool),
		tariffs:       postgres.NewTariffRepository(pool),
		insurers:      postgres.NewInsurerRepository(pool),
		professionals: users,
		tenantA:       scratchTenantA,
		tenantB:       scratchTenantB,
		appDSN:        tempAppDSN,
	}
}

func (e tariffEnv) create(t *testing.T, tenantID string, subject pricing.Tariff) error {
	t.Helper()
	return e.transactor.WithTenant(context.Background(), tenantID, func(ctx context.Context) error {
		return e.tariffs.Create(ctx, subject)
	})
}

func (e tariffEnv) newMutual(t *testing.T, tenantID, name string) insurer.Insurer {
	t.Helper()
	subject, err := insurer.New(insurer.ID(uuid.NewString()), tenant.ID(tenantID), insurer.Draft{Name: name}, time.Now())
	if err != nil {
		t.Fatalf("cannot build the mutual: %v", err)
	}
	if err := e.transactor.WithTenant(context.Background(), tenantID, func(ctx context.Context) error {
		return e.insurers.Create(ctx, subject)
	}); err != nil {
		t.Fatalf("cannot persist the mutual: %v", err)
	}
	return subject
}

func (e tariffEnv) newParticular(t *testing.T, tenantID, professional string, day time.Time, patientMinor int64) pricing.Tariff {
	t.Helper()
	subject, err := pricing.New(pricing.ID(uuid.NewString()), tenant.ID(tenantID), pricing.Draft{
		ProfessionalUserID: professional,
		Coverage:           patient.CoveragePrivate,
		Modality:           schedule.ModalityPresencial,
		Currency:           patient.CurrencyARS,
		ValidFrom:          day,
		PatientMinor:       patientMinor,
	}, time.Now())
	if err != nil {
		t.Fatalf("cannot build the tariff: %v", err)
	}
	return subject
}

func (e tariffEnv) newMutualPrice(t *testing.T, tenantID, professional, insurerID string, day time.Time) pricing.Tariff {
	t.Helper()
	subject, err := pricing.New(pricing.ID(uuid.NewString()), tenant.ID(tenantID), pricing.Draft{
		ProfessionalUserID: professional,
		Coverage:           patient.CoverageInsurer,
		InsurerID:          &insurerID,
		Modality:           schedule.ModalityOnline,
		Currency:           patient.CurrencyEUR,
		ValidFrom:          day,
		InsurerMinor:       800_000,
		PatientMinor:       150_000,
	}, time.Now())
	if err != nil {
		t.Fatalf("cannot build the mutual tariff: %v", err)
	}
	return subject
}

var november1 = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func TestTariffRepository(t *testing.T) {
	env := newTariffEnv(t)
	ctx := context.Background()
	professionalA := env.professionals[env.tenantA]
	professionalB := env.professionals[env.tenantB]

	t.Run("a particular price persists and reads back with its split", func(t *testing.T) {
		subject := env.newParticular(t, env.tenantA, professionalA, november1, 1_500_000)
		if err := env.create(t, env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}

		err := env.transactor.WithTenant(ctx, env.tenantA, func(ctx context.Context) error {
			got, err := env.tariffs.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.Coverage != patient.CoveragePrivate || got.InsurerID != nil {
				t.Errorf("coverage = %q insurer = %v, want a particular price", got.Coverage, got.InsurerID)
			}
			if got.InsurerMinor != 0 || got.PatientMinor != 1_500_000 {
				t.Errorf("split = %d/%d, want 0/1500000", got.InsurerMinor, got.PatientMinor)
			}
			if got.ValidFrom.Format("2006-01-02") != "2026-11-01" {
				t.Errorf("valid from = %v, want the civil day", got.ValidFrom)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
	})

	t.Run("a mutual price lists with its professional's and mutual's names", func(t *testing.T) {
		mutual := env.newMutual(t, env.tenantA, "OSDE Listado")
		subject := env.newMutualPrice(t, env.tenantA, professionalA, string(mutual.ID), november1.AddDate(0, 0, 10))
		if err := env.create(t, env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}

		var found *struct{ professional, insurer string }
		err := env.transactor.WithTenant(ctx, env.tenantA, func(ctx context.Context) error {
			items, err := env.tariffs.List(ctx)
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Tariff.ID == subject.ID {
					found = &struct{ professional, insurer string }{item.ProfessionalName, item.InsurerName}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if found == nil {
			t.Fatal("the mutual price is missing from the list")
		}
		if found.professional == "" || found.insurer != "OSDE Listado" {
			t.Fatalf("names = %+v, want the professional's name and the mutual's", found)
		}
	})

	t.Run("a second price for the same combination and day is refused", func(t *testing.T) {
		first := env.newParticular(t, env.tenantA, professionalA, november1.AddDate(0, 0, 20), 1_000_000)
		if err := env.create(t, env.tenantA, first); err != nil {
			t.Fatalf("first: %v", err)
		}
		second := env.newParticular(t, env.tenantA, professionalA, november1.AddDate(0, 0, 20), 2_000_000)
		if err := env.create(t, env.tenantA, second); !errors.Is(err, pricing.ErrValidityTaken) {
			t.Fatalf("second: error = %v, want ErrValidityTaken (NULLS NOT DISTINCT makes the particular rows collide)", err)
		}
	})

	t.Run("a later price for the same combination is a new version", func(t *testing.T) {
		later := env.newParticular(t, env.tenantA, professionalA, november1.AddDate(0, 1, 0), 2_000_000)
		if err := env.create(t, env.tenantA, later); err != nil {
			t.Fatalf("a later day must coexist: %v", err)
		}
	})

	t.Run("a professional of another practice is refused as a field error", func(t *testing.T) {
		subject := env.newParticular(t, env.tenantA, professionalB, november1.AddDate(0, 0, 30), 1_000_000)
		var validation *pricing.ValidationError
		err := env.create(t, env.tenantA, subject)
		if !errors.As(err, &validation) || validation.Field != "professional_user_id" {
			t.Fatalf("error = %v, want a professional_user_id validation error", err)
		}
	})

	t.Run("a mutual of another practice is refused as a field error", func(t *testing.T) {
		foreign := env.newMutual(t, env.tenantB, "OSDE Ajena")
		subject := env.newMutualPrice(t, env.tenantA, professionalA, string(foreign.ID), november1.AddDate(0, 0, 40))
		var validation *pricing.ValidationError
		err := env.create(t, env.tenantA, subject)
		if !errors.As(err, &validation) || validation.Field != "insurer_id" {
			t.Fatalf("error = %v, want an insurer_id validation error", err)
		}
	})

	t.Run("another practice neither reads nor changes a price", func(t *testing.T) {
		subject := env.newParticular(t, env.tenantA, professionalA, november1.AddDate(0, 0, 50), 1_000_000)
		if err := env.create(t, env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}

		err := env.transactor.WithTenant(ctx, env.tenantB, func(ctx context.Context) error {
			items, err := env.tariffs.List(ctx)
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Tariff.TenantID == tenant.ID(env.tenantA) {
					t.Errorf("practice B lists practice A's price %s", item.Tariff.ID)
				}
			}
			if _, err := env.tariffs.ByID(ctx, subject.ID); !errors.Is(err, pricing.ErrNotFound) {
				t.Errorf("ByID across practices = %v, want ErrNotFound", err)
			}
			subject.PatientMinor = 1
			if err := env.tariffs.Update(ctx, subject); !errors.Is(err, pricing.ErrNotFound) {
				t.Errorf("Update across practices = %v, want ErrNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cross-practice reads: %v", err)
		}
	})

	t.Run("a write outside a practice scope is refused before any statement", func(t *testing.T) {
		subject := env.newParticular(t, env.tenantA, professionalA, november1.AddDate(0, 0, 60), 1_000_000)
		if err := env.tariffs.Create(ctx, subject); !errors.Is(err, pricing.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want ErrTenantScopeMissing", err)
		}
	})

	t.Run("amounts change and a removed price is not found again", func(t *testing.T) {
		subject := env.newParticular(t, env.tenantA, professionalA, november1.AddDate(0, 0, 70), 1_000_000)
		if err := env.create(t, env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}

		updated, err := subject.ChangeAmounts(pricing.Amounts{PatientMinor: 1_300_000}, time.Now())
		if err != nil {
			t.Fatalf("change amounts: %v", err)
		}
		err = env.transactor.WithTenant(ctx, env.tenantA, func(ctx context.Context) error {
			if err := env.tariffs.Update(ctx, updated); err != nil {
				return err
			}
			got, err := env.tariffs.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.PatientMinor != 1_300_000 {
				t.Errorf("patient minor = %d, want 1300000", got.PatientMinor)
			}
			if err := env.tariffs.Delete(ctx, subject.ID); err != nil {
				return err
			}
			return env.tariffs.Delete(ctx, subject.ID)
		})
		if !errors.Is(err, pricing.ErrNotFound) {
			t.Fatalf("second delete = %v, want ErrNotFound", err)
		}
	})
}

// TestTariffSchemaRefusesBrokenRows proves the database enforces the split rules on its
// own, for a writer that bypasses the domain.
func TestTariffSchemaRefusesBrokenRows(t *testing.T) {
	env := newTariffEnv(t)
	ctx := context.Background()
	professional := env.professionals[env.tenantA]

	conn, err := pgx.Connect(ctx, env.appDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	insert := func(coverage string, insurerID *string, insurerMinor, patientMinor int64, modality string, currency string) error {
		return withTenantSetting(ctx, conn, env.tenantA, func(ctx context.Context) error {
			_, err := conn.Exec(ctx, `
				INSERT INTO tariffs (id, tenant_id, professional_user_id, coverage, insurer_id, modality,
				                     billing_currency, valid_from, insurer_minor, patient_minor, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, '2026-11-01', $8, $9, now(), now())`,
				uuid.NewString(), env.tenantA, professional, coverage, insurerID, modality, currency, insurerMinor, patientMinor)
			return err
		})
	}
	expectCode := func(t *testing.T, err error, code string) {
		t.Helper()
		var pgErr interface{ SQLState() string }
		if !errors.As(err, &pgErr) || pgErr.SQLState() != code {
			t.Fatalf("error = %v, want SQLSTATE %s", err, code)
		}
	}

	t.Run("a particular price cannot carry a mutual share", func(t *testing.T) {
		expectCode(t, insert("private", nil, 100, 900, "presencial", "ARS"), "23514")
	})
	t.Run("a session worth nothing is refused", func(t *testing.T) {
		expectCode(t, insert("private", nil, 0, 0, "presencial", "ARS"), "23514")
	})
	t.Run("a mutual price must name its mutual", func(t *testing.T) {
		expectCode(t, insert("insurer", nil, 800, 200, "online", "ARS"), "23514")
	})
	t.Run("an unknown modality or currency is refused", func(t *testing.T) {
		expectCode(t, insert("private", nil, 0, 900, "hibrido", "ARS"), "23514")
		expectCode(t, insert("private", nil, 0, 900, "online", "BRL"), "23514")
	})
	t.Run("a well-formed particular price is accepted", func(t *testing.T) {
		if err := insert("private", nil, 0, 900, "online", "USD"); err != nil {
			t.Fatalf("a valid row: %v", err)
		}
	})
}

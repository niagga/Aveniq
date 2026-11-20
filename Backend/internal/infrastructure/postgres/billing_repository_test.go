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
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

func billingEnv(t *testing.T) (*postgres.Transactor, *postgres.BillingRepository, map[string]string, map[string]string) {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the billing tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)
	patients, users := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

	pool, err := pgxpool.New(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database as the application role: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return postgres.NewTransactor(pool), postgres.NewBillingRepository(pool), patients, users
}

func TestBillingRepository(t *testing.T) {
	transactor, repo, patients, users := billingEnv(t)
	ctx := context.Background()

	tenantA := scratchTenantA
	tenantB := scratchTenantB
	patientA := patients[tenantA]
	patientB := patients[tenantB]
	issuerA := users[tenantA]
	now := time.Date(2026, 11, 15, 16, 0, 0, 0, time.UTC)

	issue := func(t *testing.T, tenantID, issuerID string, draft billing.IssueDraft) billing.Voucher {
		t.Helper()
		built, err := billing.NewIssued(
			billing.VoucherID(uuid.NewString()),
			tenant.ID(tenantID),
			issuerID,
			draft,
			now,
		)
		if err != nil {
			t.Fatalf("cannot build the voucher: %v", err)
		}
		var saved billing.Voucher
		if err := transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			var inner error
			saved, inner = repo.Issue(ctx, built)
			return inner
		}); err != nil {
			t.Fatalf("cannot issue: %v", err)
		}
		return saved
	}

	loose := func() billing.IssueDraft {
		return billing.IssueDraft{
			Concept:      "Consulta suelta",
			AmountMinor:  150000,
			Currency:     "ARS",
			PracticeName: "IAM A",
		}
	}

	t.Run("numbers are consecutive and a void does not reuse them", func(t *testing.T) {
		first := issue(t, tenantA, issuerA, loose())
		if first.Number != 1 {
			t.Fatalf("first number = %d, want 1", first.Number)
		}
		second := issue(t, tenantA, issuerA, loose())
		if second.Number != 2 {
			t.Fatalf("second number = %d, want 2", second.Number)
		}

		voided, err := first.Void(issuerA, "Error de carga", now.Add(time.Minute))
		if err != nil {
			t.Fatalf("domain void: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			voided, inner = repo.Void(ctx, voided)
			return inner
		}); err != nil {
			t.Fatalf("cannot void: %v", err)
		}
		if voided.Number != 1 || voided.Status != billing.StatusVoided {
			t.Fatalf("voided = number %d status %q, want number 1 voided", voided.Number, voided.Status)
		}

		third := issue(t, tenantA, issuerA, loose())
		if third.Number != 3 {
			t.Fatalf("after a void the next number = %d, want 3", third.Number)
		}
	})

	t.Run("a failed issue leaves no gap", func(t *testing.T) {
		before := issue(t, tenantA, issuerA, loose())

		intruder, err := billing.NewIssued(
			billing.VoucherID(uuid.NewString()),
			tenant.ID(tenantA),
			issuerA,
			billing.IssueDraft{
				PatientID:    patientB,
				Concept:      "Sesión ajena",
				AmountMinor:  1000,
				Currency:     "ARS",
				PracticeName: "IAM A",
				BuyerName:    strPtr("Ajena"),
				Photo:        billing.Photo{Coverage: billing.CoveragePrivate},
			},
			now,
		)
		if err != nil {
			t.Fatalf("cannot build the intruder: %v", err)
		}
		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, inner := repo.Issue(ctx, intruder)
			return inner
		})
		if !errors.Is(err, billing.ErrPatientNotVisible) {
			t.Fatalf("error = %v, want %v", err, billing.ErrPatientNotVisible)
		}

		after := issue(t, tenantA, issuerA, loose())
		if after.Number != before.Number+1 {
			t.Fatalf("after a rolled-back issue the next number = %d, want %d", after.Number, before.Number+1)
		}
	})

	t.Run("the photo is stored, read back and not rewritten by a void", func(t *testing.T) {
		photo := billing.Photo{ProfessionalID: issuerA, ProfessionalName: "Ana Profesional"}
		saved := issue(t, tenantA, issuerA, billing.IssueDraft{
			Concept:      "Cargo suelto con profesional",
			AmountMinor:  1000,
			Currency:     "ARS",
			PracticeName: "IAM A",
			Photo:        photo,
		})

		read := func(id billing.VoucherID) billing.Voucher {
			t.Helper()
			var voucher billing.Voucher
			if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
				var inner error
				voucher, inner = repo.ByID(ctx, id)
				return inner
			}); err != nil {
				t.Fatalf("cannot read %s: %v", id, err)
			}
			return voucher
		}

		if got := read(saved.ID).Photo; got != photo {
			t.Fatalf("stored photo = %+v, want %+v", got, photo)
		}

		voided, err := saved.Void(issuerA, "Error de carga", now.Add(time.Minute))
		if err != nil {
			t.Fatalf("domain void: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			voided, inner = repo.Void(ctx, voided)
			return inner
		}); err != nil {
			t.Fatalf("cannot void: %v", err)
		}
		if got := read(saved.ID).Photo; got != photo {
			t.Fatalf("void rewrote the photo: %+v, want %+v", got, photo)
		}
	})

	t.Run("the buyer snapshot is stored and not rewritten by a void", func(t *testing.T) {
		name := "Ana Pérez"
		draft := billing.IssueDraft{
			PatientID:    patientA,
			Concept:      "Sesión",
			AmountMinor:  200000,
			Currency:     "ARS",
			PracticeName: "IAM A",
			BuyerName:    &name,
			Photo:        billing.Photo{Coverage: billing.CoveragePrivate},
		}
		saved := issue(t, tenantA, issuerA, draft)
		if saved.BuyerName == nil || *saved.BuyerName != "Ana Pérez" {
			t.Fatalf("BuyerName = %v, want the snapshot", saved.BuyerName)
		}

		voided, err := saved.Void(issuerA, "Carga duplicada", now.Add(time.Minute))
		if err != nil {
			t.Fatalf("domain void: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			voided, inner = repo.Void(ctx, voided)
			return inner
		}); err != nil {
			t.Fatalf("cannot void: %v", err)
		}
		if voided.BuyerName == nil || *voided.BuyerName != "Ana Pérez" || voided.PracticeName != "IAM A" {
			t.Fatalf("void rewrote the snapshot: buyer=%v practice=%q", voided.BuyerName, voided.PracticeName)
		}
	})

	t.Run("vouchers are invisible from another practice", func(t *testing.T) {
		saved := issue(t, tenantA, issuerA, loose())

		err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			_, err := repo.ByID(ctx, saved.ID)
			return err
		})
		if !errors.Is(err, billing.ErrNotFound) {
			t.Fatalf("ByID from another tenant: error = %v, want %v", err, billing.ErrNotFound)
		}

		if err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			items, err := repo.List(ctx, port.VoucherFilter{})
			if err != nil {
				return err
			}
			if len(items) != 0 {
				t.Fatalf("list from another tenant = %d, want 0", len(items))
			}
			return nil
		}); err != nil {
			t.Fatalf("list isolation: %v", err)
		}

		voided, err := saved.Void(issuerA, "Ajena", now.Add(time.Minute))
		if err != nil {
			t.Fatalf("domain void: %v", err)
		}
		err = transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			_, inner := repo.Void(ctx, voided)
			return inner
		})
		if !errors.Is(err, billing.ErrNotFound) {
			t.Fatalf("void from another tenant: error = %v, want %v", err, billing.ErrNotFound)
		}
	})

	t.Run("a second void is refused", func(t *testing.T) {
		saved := issue(t, tenantA, issuerA, loose())
		voided, err := saved.Void(issuerA, "Error", now.Add(time.Minute))
		if err != nil {
			t.Fatalf("domain void: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, inner := repo.Void(ctx, voided)
			return inner
		}); err != nil {
			t.Fatalf("first void: %v", err)
		}
		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, inner := repo.Void(ctx, voided)
			return inner
		})
		if !errors.Is(err, billing.ErrAlreadyVoided) {
			t.Fatalf("second void: error = %v, want %v", err, billing.ErrAlreadyVoided)
		}
	})

	t.Run("issue without a tenant transaction is refused", func(t *testing.T) {
		built, err := billing.NewIssued(
			billing.VoucherID(uuid.NewString()),
			tenant.ID(tenantA),
			issuerA,
			loose(),
			now,
		)
		if err != nil {
			t.Fatalf("cannot build: %v", err)
		}
		if _, err := repo.Issue(ctx, built); !errors.Is(err, billing.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want %v", err, billing.ErrTenantScopeMissing)
		}
	})
}

func strPtr(value string) *string { return &value }

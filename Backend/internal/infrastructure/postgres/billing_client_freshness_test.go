package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/report"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// TestBillingReadsFollowTheClient proves the defect L91 recorded is gone: the fiscal data
// and the currency billing and the reports answer with belong to the client (S26), so
// after the client is edited every read answers with the edited value. The patient row
// keeps the mirror it was born with; if any of these reads still hit it, the stale value
// comes back, and that is exactly what each check says out loud.
func TestBillingReadsFollowTheClient(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the client-following billing tests")
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
	t.Cleanup(pool.Close)

	seeder, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = seeder.Close(ctx) }()

	transactor := postgres.NewTransactor(pool)
	patientRepo := postgres.NewPatientRepository(pool)
	clients := postgres.NewClientRepository(pool)
	catalog := postgres.NewPatientCatalog(pool)
	billingRepo := postgres.NewBillingRepository(pool)
	reports := postgres.NewReportRepository(pool)

	tenantA := scratchTenantA

	// Born the ordinary way: the client is created with the same data the patient
	// mirrors, which is what makes a stale read invisible at birth.
	ana := newPatient(t, catalog, tenantA, "Ana Seguidora", "30.111.222")
	if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		return persistPatient(ctx, clients, patientRepo, ana)
	}); err != nil {
		t.Fatalf("cannot create the patient: %v", err)
	}
	clientID := clientOf(t, pool, tenantA, string(ana.ID))

	// One attended session in October, unbilled: the report has something to value.
	sessionID := "1e0c2a12-1111-4222-8333-000000000001"
	if err := withTenantSetting(ctx, seeder, tenantA, func(ctx context.Context) error {
		_, err := seeder.Exec(ctx, `
			INSERT INTO appointments (id, tenant_id, professional_user_id, patient_id, starts_at, ends_at, status, modality, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'attended', 'presencial', now(), now())`,
			sessionID, tenantA, users[tenantA], ana.ID,
			time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 6, 15, 45, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatalf("cannot insert the attended session: %v", err)
	}

	searchSubjects := func(name, fragment string) billing.Subject {
		t.Helper()
		var found []billing.Subject
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			found, inner = billingRepo.SearchSubjects(ctx, fragment, 20)
			return inner
		}); err != nil {
			t.Fatalf("%s: the search disagreed: %v", name, err)
		}
		if len(found) != 1 {
			t.Fatalf("%s: the search for %q returned %d subjects, want exactly %q", name, fragment, len(found), ana.FullName)
		}
		if found[0].FullName != ana.FullName {
			t.Fatalf("%s: the search for %q returned %q, want %q", name, fragment, found[0].FullName, ana.FullName)
		}
		return found[0]
	}

	// Before the edit the client and the mirror agree, so this pins the baseline.
	t.Run("before the edit the search answers with the shared data", func(t *testing.T) {
		subject := searchSubjects("baseline", "20123456786")
		if subject.TaxID == nil || *subject.TaxID != "20-12345678-6" {
			t.Errorf("baseline TaxID = %v, want the CUIT 20-12345678-6 the client was born with", subject.TaxID)
		}
		if subject.FiscalCondition == nil || *subject.FiscalCondition != "monotributo" {
			t.Errorf("baseline FiscalCondition = %v, want monotributo", subject.FiscalCondition)
		}
		if subject.BillingCurrency != "ARS" {
			t.Errorf("baseline BillingCurrency = %q, want the ARS the client was born with", subject.BillingCurrency)
		}
	})

	// The payer is edited through the real client repository, the way UpdateClient
	// does: fiscal condition, tax id and currency all change. The patient row is not
	// touched, so from here on the mirror disagrees with the client on all three.
	newCUIT := patient.TaxID{Kind: "cuit", Number: "27-25874639-8", Normalized: "27258746398"}
	responsible := "responsable_inscripto"
	if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		current, err := clients.ByID(ctx, client.ID(clientID))
		if err != nil {
			return err
		}
		edited, err := current.ChangeProfile(client.Draft{
			DisplayName:     current.DisplayName,
			LegalName:       current.LegalName,
			Country:         current.Country,
			TaxID:           &newCUIT,
			FiscalCondition: &responsible,
			FiscalAddress:   current.FiscalAddress,
			BillingCurrency: patient.CurrencyUSD,
		}, time.Now().UTC())
		if err != nil {
			return err
		}
		return clients.Update(ctx, edited)
	}); err != nil {
		t.Fatalf("cannot edit the client: %v", err)
	}

	t.Run("after the edit the subject search answers with the client's data", func(t *testing.T) {
		subject := searchSubjects("edited tax id", "258746398")
		if subject.TaxID == nil || *subject.TaxID != newCUIT.Number {
			t.Errorf("the search answered TaxID = %v, want the edited CUIT %q; if it shows the birth CUIT 20-12345678-6 the search still reads the patient's mirror", subject.TaxID, newCUIT.Number)
		}
		if subject.FiscalCondition == nil || *subject.FiscalCondition != responsible {
			t.Errorf("the search answered FiscalCondition = %v, want %q; if it shows monotributo the search still reads the patient's mirror", subject.FiscalCondition, responsible)
		}
		if subject.BillingCurrency != "USD" {
			t.Errorf("the search answered BillingCurrency = %q, want the edited USD; if it shows ARS the search still reads the patient's mirror", subject.BillingCurrency)
		}
		// And the birth CUIT no longer matches: the mirror is dead to billing.
		var stale []billing.Subject
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			stale, inner = billingRepo.SearchSubjects(ctx, "20123456786", 20)
			return inner
		}); err != nil {
			t.Fatalf("the search for the stale CUIT: %v", err)
		}
		if len(stale) != 0 {
			t.Errorf("the search for the stale birth CUIT 20-12345678-6 still returns %q: the identifier is matched against the patient's mirror, not the client", stale[0].FullName)
		}
	})

	t.Run("after the edit the patient's billing terms carry the client's currency", func(t *testing.T) {
		var terms billing.PatientBilling
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			terms, inner = billingRepo.PatientBilling(ctx, string(ana.ID))
			return inner
		}); err != nil {
			t.Fatalf("cannot read the billing terms: %v", err)
		}
		if terms.Currency != "USD" {
			t.Errorf("PatientBilling.Currency = %q, want the client's edited USD; if it shows ARS the read still hits the patient's mirror", terms.Currency)
		}
		if terms.Coverage != billing.CoveragePrivate {
			t.Errorf("PatientBilling.Coverage = %q, want the patient's private coverage (S26: the mutual stays on the patient)", terms.Coverage)
		}
	})

	t.Run("after the edit the report values the session in the client's currency", func(t *testing.T) {
		window := report.Window{
			From: time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC),
			To:   time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC),
		}
		var lines []report.Line
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			lines, inner = reports.PendingSessions(ctx, window, report.Filter{})
			return inner
		}); err != nil {
			t.Fatalf("cannot read the pending sessions: %v", err)
		}
		if len(lines) != 1 {
			t.Fatalf("pending sessions = %d, want exactly the one attended session", len(lines))
		}
		if lines[0].Currency != "USD" {
			t.Errorf("the report valued the session in %q, want the client's edited USD; if it shows ARS the valuation still comes from the patient's mirror", lines[0].Currency)
		}
	})

	t.Run("the issue path still resolves the payer of the patient", func(t *testing.T) {
		var subject billing.Subject
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			subject, inner = billingRepo.Subject(ctx, string(ana.ID))
			return inner
		}); err != nil {
			t.Fatalf("cannot read the billing subject: %v", err)
		}
		if subject.PatientID != string(ana.ID) {
			t.Errorf("Subject.PatientID = %q, want %q", subject.PatientID, ana.ID)
		}
		if subject.ClientID != clientID {
			t.Errorf("Subject.ClientID = %q, want the payer %q", subject.ClientID, clientID)
		}
	})
}

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

	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/report"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// TestReportRepository runs the two reads of the billing report through the adapter, against the
// real schema: the issued vouchers of a window with the photo they were issued with, and the
// attended sessions that no issued voucher bills.
func TestReportRepository(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the report repository tests")
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
	transactor := postgres.NewTransactor(pool)
	repo := postgres.NewReportRepository(pool)

	seeder, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = seeder.Close(ctx) }()

	tenantA, tenantB := scratchTenantA, scratchTenantB
	professional := users[tenantA]
	private := patients[tenantA]
	covered := uuid.NewString()
	insurerID := uuid.NewString()

	billedSession := uuid.NewString() // covered, October, billed by an issued voucher
	onlineSession := uuid.NewString() // covered, October, online, not billed
	voidedSession := uuid.NewString() // private, October, its voucher was voided
	septemberSess := uuid.NewString() // private, September, not billed
	scheduledSess := uuid.NewString() // private, October, never attended
	attended := map[string]struct {
		patient  string
		status   string
		modality string
		at       time.Time
	}{
		billedSession: {covered, "attended", "presencial", time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)},
		onlineSession: {covered, "attended", "online", time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC)},
		voidedSession: {private, "attended", "presencial", time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC)},
		septemberSess: {private, "attended", "presencial", time.Date(2026, 9, 28, 15, 0, 0, 0, time.UTC)},
		scheduledSess: {private, "scheduled", "presencial", time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)},
	}

	if err := withTenantSetting(ctx, seeder, tenantA, func(ctx context.Context) error {
		if _, err := seeder.Exec(ctx, `
			INSERT INTO insurers (id, tenant_id, name, tax_id, created_at, updated_at)
			VALUES ($1, $2, 'OSDE', '30711223334', now(), now())`, insurerID, tenantA); err != nil {
			return err
		}
		if schemaPatientsHaveClientID(ctx, seeder) {
			clientID, err := insertClientApp(ctx, seeder, tenantA, "Ana Mutual", "AR")
			if err != nil {
				return err
			}
			if _, err := seeder.Exec(ctx, `
				INSERT INTO patients (id, tenant_id, client_id, country, full_name, coverage_kind, insurer_id, member_number, billing_currency, created_at, updated_at)
				VALUES ($1, $2, $3, 'AR', 'Ana Mutual', 'insurer', $4, 'AF-1', 'ARS', now(), now())`, covered, tenantA, clientID, insurerID); err != nil {
				return err
			}
		} else if _, err := seeder.Exec(ctx, `
			INSERT INTO patients (id, tenant_id, country, full_name, coverage_kind, insurer_id, member_number, billing_currency, created_at, updated_at)
			VALUES ($1, $2, 'AR', 'Ana Mutual', 'insurer', $3, 'AF-1', 'ARS', now(), now())`, covered, tenantA, insurerID); err != nil {
			return err
		}
		for id, session := range attended {
			if _, err := seeder.Exec(ctx, `
				INSERT INTO appointments (id, tenant_id, professional_user_id, patient_id, starts_at, ends_at, status, modality, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())`,
				id, tenantA, professional, session.patient, session.at, session.at.Add(45*time.Minute), session.status, session.modality); err != nil {
				return err
			}
		}

		// v1 bills the covered patient's session, with the photo of a mutual.
		if _, err := seeder.Exec(ctx, `
			INSERT INTO vouchers (
				id, tenant_id, number, patient_id, appointment_id, concept, amount_minor, currency,
				practice_name, buyer_name, coverage_kind, insurer_id, insurer_name, modality,
				professional_user_id, professional_name, issued_by, issued_at, created_at, updated_at
			) VALUES (
				$1, $2, 1, $3, $4, 'Sesión', 950000, 'ARS', 'Consultorio Test', 'Ana Mutual',
				'insurer', $5, 'OSDE', 'presencial', $6, 'Demo Profesional', $6, $7, now(), now())`,
			uuid.NewString(), tenantA, covered, billedSession, insurerID, professional,
			time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)); err != nil {
			return err
		}
		// v2 was voided: it bills nothing and frees its session.
		if _, err := seeder.Exec(ctx, `
			INSERT INTO vouchers (
				id, tenant_id, number, patient_id, appointment_id, concept, amount_minor, currency,
				practice_name, buyer_name, coverage_kind, modality, professional_user_id, professional_name,
				issued_by, issued_at, status, voided_by, voided_at, void_reason, created_at, updated_at
			) VALUES (
				$1, $2, 2, $3, $4, 'Sesión', 150000, 'ARS', 'Consultorio Test', 'Paciente',
				'private', 'presencial', $5, 'Demo Profesional', $5, $6, 'voided', $5, $7, 'Error de carga', now(), now())`,
			uuid.NewString(), tenantA, private, voidedSession, professional,
			time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC), time.Date(2026, 10, 8, 18, 0, 0, 0, time.UTC)); err != nil {
			return err
		}
		// v3 is a loose charge in dollars: no patient, no photo.
		_, err := seeder.Exec(ctx, `
			INSERT INTO vouchers (
				id, tenant_id, number, concept, amount_minor, currency, practice_name, issued_by, issued_at, created_at, updated_at
			) VALUES ($1, $2, 3, 'Supervisión', 5000, 'USD', 'Consultorio Test', $3, $4, now(), now())`,
			uuid.NewString(), tenantA, professional, time.Date(2026, 10, 20, 18, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	october := report.Window{From: time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)}
	september := report.Window{From: time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC), To: time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)}

	billed := func(t *testing.T, tenantID string, window report.Window, filter report.Filter) []report.BilledLine {
		t.Helper()
		var got []report.BilledLine
		if err := transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			var inner error
			got, inner = repo.BilledLines(ctx, window, filter)
			return inner
		}); err != nil {
			t.Fatalf("read the billed vouchers: %v", err)
		}
		return got
	}
	pending := func(t *testing.T, tenantID string, window report.Window, filter report.Filter) []report.Line {
		t.Helper()
		var got []report.Line
		if err := transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			var inner error
			got, inner = repo.PendingSessions(ctx, window, filter)
			return inner
		}); err != nil {
			t.Fatalf("read the pending sessions: %v", err)
		}
		return got
	}

	t.Run("the billed lines are the issued vouchers of the window, with their photo", func(t *testing.T) {
		got := billed(t, tenantA, october, report.Filter{})
		if len(got) != 2 {
			t.Fatalf("billed = %d, want 2 (the voided voucher bills nothing): %+v", len(got), got)
		}
		mutual, loose := got[0], got[1]
		if mutual.AmountMinor != 950000 || mutual.Currency != "ARS" || mutual.Coverage != "insurer" ||
			mutual.InsurerID != insurerID || mutual.InsurerName != "OSDE" || mutual.Modality != "presencial" ||
			mutual.ProfessionalID != professional || mutual.ProfessionalName != "Demo Profesional" {
			t.Errorf("the mutual's voucher = %+v", mutual)
		}
		if loose.AmountMinor != 5000 || loose.Currency != "USD" || loose.Coverage != "" || loose.InsurerID != "" || loose.ProfessionalID != "" {
			t.Errorf("the loose charge = %+v, want USD with no photo", loose)
		}
		if len(billed(t, tenantA, september, report.Filter{})) != 0 {
			t.Errorf("September has no vouchers")
		}
	})

	t.Run("a filter on a cut leaves out the vouchers that carry no photo of it", func(t *testing.T) {
		for name, filter := range map[string]report.Filter{
			"coverage":     {Coverage: "insurer"},
			"insurer":      {InsurerID: insurerID},
			"modality":     {Modality: "presencial"},
			"professional": {ProfessionalID: professional},
		} {
			got := billed(t, tenantA, october, filter)
			if len(got) != 1 || got[0].AmountMinor != 950000 {
				t.Errorf("filter %s = %+v, want only the mutual's voucher", name, got)
			}
		}
		if got := billed(t, tenantA, october, report.Filter{Modality: "online"}); len(got) != 0 {
			t.Errorf("online = %+v, want none", got)
		}
		if got := billed(t, tenantA, october, report.Filter{ProfessionalID: uuid.NewString()}); len(got) != 0 {
			t.Errorf("another professional = %+v, want none", got)
		}
	})

	t.Run("the pending sessions are the attended ones no issued voucher bills", func(t *testing.T) {
		got := pending(t, tenantA, october, report.Filter{})
		if len(got) != 2 {
			t.Fatalf("pending = %d, want 2 (the online one and the one whose voucher was voided): %+v", len(got), got)
		}
		online, voided := got[0], got[1]
		if online.Modality != "online" || online.Coverage != "insurer" || online.InsurerID != insurerID ||
			online.InsurerName != "OSDE" || online.Currency != "ARS" || online.ProfessionalID != professional ||
			online.ProfessionalName == "" || !online.At.Equal(attended[onlineSession].at) {
			t.Errorf("the online session = %+v", online)
		}
		if voided.Coverage != "private" || voided.InsurerID != "" || voided.InsurerName != "" || !voided.At.Equal(attended[voidedSession].at) {
			t.Errorf("the session of the voided voucher = %+v, want private with no mutual", voided)
		}

		if got := pending(t, tenantA, september, report.Filter{}); len(got) != 1 || !got[0].At.Equal(attended[septemberSess].at) {
			t.Errorf("September pending = %+v, want only the September session", got)
		}
		if got := pending(t, tenantA, october, report.Filter{Coverage: "private"}); len(got) != 1 || got[0].Coverage != "private" {
			t.Errorf("private pending = %+v, want 1", got)
		}
		if got := pending(t, tenantA, october, report.Filter{InsurerID: insurerID}); len(got) != 1 || got[0].Modality != "online" {
			t.Errorf("OSDE pending = %+v, want the online session", got)
		}
		if got := pending(t, tenantA, october, report.Filter{Modality: "presencial"}); len(got) != 1 {
			t.Errorf("presencial pending = %+v, want 1", got)
		}
		if got := pending(t, tenantA, october, report.Filter{ProfessionalID: uuid.NewString()}); len(got) != 0 {
			t.Errorf("another professional pending = %+v, want none", got)
		}
	})

	t.Run("another practice reads none of it", func(t *testing.T) {
		if got := billed(t, tenantB, october, report.Filter{}); len(got) != 0 {
			t.Errorf("practice B billed = %+v, want none", got)
		}
		if got := pending(t, tenantB, october, report.Filter{}); len(got) != 0 {
			t.Errorf("practice B pending = %+v, want none", got)
		}
	})

	t.Run("a read outside a tenant transaction is refused", func(t *testing.T) {
		if _, err := repo.BilledLines(ctx, october, report.Filter{}); !errors.Is(err, billing.ErrTenantScopeMissing) {
			t.Errorf("billed without a scope: error = %v, want %v", err, billing.ErrTenantScopeMissing)
		}
		if _, err := repo.PendingSessions(ctx, october, report.Filter{}); !errors.Is(err, billing.ErrTenantScopeMissing) {
			t.Errorf("pending without a scope: error = %v, want %v", err, billing.ErrTenantScopeMissing)
		}
	})
}

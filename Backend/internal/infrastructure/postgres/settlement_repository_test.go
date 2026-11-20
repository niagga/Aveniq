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

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// TestSettlementRepository runs the monthly settlement through the adapter: the mutual is read,
// its attended sessions are listed for the month, the settlement is issued and voided, and the
// voided sessions can be listed again.
func TestSettlementRepository(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL and TEST_APP_DATABASE_URL to run the settlement repository tests")
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
	transactor := postgres.NewTransactor(pool)
	repo := postgres.NewSettlementRepository(pool)

	seeder, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = seeder.Close(ctx) }()

	tenantA := scratchTenantA
	tenantB := scratchTenantB
	covered := uuid.NewString()
	insurerID := uuid.NewString()
	taxID := "30711223334"
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

	// The mutual with its CUIT, a patient it covers, and three attended sessions of that patient:
	// one in September, which the October settlement must not take.
	sessions := map[string]time.Time{
		uuid.NewString(): time.Date(2026, 9, 28, 15, 0, 0, 0, time.UTC),
		uuid.NewString(): time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC),
		uuid.NewString(): time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC),
	}
	septemberSession := ""
	var octoberSessions []string
	for id, startsAt := range sessions {
		if startsAt.Month() == time.September {
			septemberSession = id
		} else {
			octoberSessions = append(octoberSessions, id)
		}
	}
	if err := withTenantSetting(ctx, seeder, tenantA, func(ctx context.Context) error {
		if _, err := seeder.Exec(ctx, `
			INSERT INTO insurers (id, tenant_id, name, tax_id, created_at, updated_at)
			VALUES ($1, $2, 'OSDE', $3, now(), now())`, insurerID, tenantA, taxID); err != nil {
			return err
		}
		if _, err := seeder.Exec(ctx, `
			INSERT INTO patients (id, tenant_id, country, full_name, coverage_kind, insurer_id, member_number, billing_currency, created_at, updated_at)
			VALUES ($1, $2, 'AR', 'Ana Mutual', 'insurer', $3, 'AF-1', 'ARS', now(), now())`, covered, tenantA, insurerID); err != nil {
			return err
		}
		for id, startsAt := range sessions {
			if _, err := seeder.Exec(ctx, `
				INSERT INTO appointments (id, tenant_id, professional_user_id, patient_id, starts_at, ends_at, status, modality, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, 'attended', 'presencial', now(), now())`,
				id, tenantA, users[tenantA], covered, startsAt, startsAt.Add(45*time.Minute)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	insurer := billing.SettlementInsurer{ID: insurerID, Name: "OSDE", TaxID: &taxID, Active: true}
	october := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	monthFrom := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	monthTo := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	item := func(appointmentID string, startsAt time.Time) billing.SettlementItem {
		return billing.SettlementItem{
			AppointmentID:    appointmentID,
			PatientID:        covered,
			PatientName:      "Ana Mutual",
			MemberNumber:     "AF-1",
			ProfessionalID:   users[tenantA],
			ProfessionalName: "Demo Profesional",
			Modality:         billing.ModalityPresencial,
			StartsAt:         startsAt,
			Currency:         "ARS",
			AmountMinor:      800000,
			CopagoMinor:      150000,
		}
	}

	var issued billing.Settlement
	t.Run("the mutual is read with its CUIT, and only inside its practice", func(t *testing.T) {
		var got billing.SettlementInsurer
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			got, inner = repo.Insurer(ctx, insurerID)
			return inner
		}); err != nil {
			t.Fatalf("read the mutual: %v", err)
		}
		if got.Name != "OSDE" || got.TaxID == nil || *got.TaxID != taxID || !got.Active {
			t.Fatalf("mutual = %+v, want OSDE with its CUIT, active", got)
		}

		err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			_, inner := repo.Insurer(ctx, insurerID)
			return inner
		})
		if !errors.Is(err, billing.ErrInsurerNotVisible) {
			t.Fatalf("practice B reads the mutual of A: error = %v, want %v", err, billing.ErrInsurerNotVisible)
		}
	})

	t.Run("the settlable sessions are the month's attended sessions of the mutual's patients", func(t *testing.T) {
		var got []billing.SettlableSession
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			got, inner = repo.SettlableSessions(ctx, insurerID, monthFrom, monthTo)
			return inner
		}); err != nil {
			t.Fatalf("list settlable sessions: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("settlable sessions = %d, want 2 (October only): %+v", len(got), got)
		}
		if got[0].StartsAt.After(got[1].StartsAt) {
			t.Errorf("sessions are not oldest first: %v before %v", got[0].StartsAt, got[1].StartsAt)
		}
		for _, session := range got {
			if session.AppointmentID == septemberSession || session.Currency != "ARS" || session.MemberNumber != "AF-1" {
				t.Errorf("session %+v is not one October session of the covered patient", session)
			}
		}
	})

	t.Run("a settlement is issued with its sessions, and they are no longer settlable", func(t *testing.T) {
		items := []billing.SettlementItem{
			item(octoberSessions[0], sessions[octoberSessions[0]]),
			item(octoberSessions[1], sessions[octoberSessions[1]]),
		}
		settlement, err := billing.NewSettlement(billing.SettlementID(uuid.NewString()), tenant.ID(tenantA), users[tenantA], "IAM A", insurer, october, items, now)
		if err != nil {
			t.Fatalf("domain issue: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			issued, inner = repo.Issue(ctx, settlement)
			return inner
		}); err != nil {
			t.Fatalf("issue: %v", err)
		}
		if issued.Status != billing.StatusIssued || len(issued.Items) != 2 || issued.InsurerTaxID != taxID {
			t.Fatalf("issued = status %q, %d items, CUIT %q", issued.Status, len(issued.Items), issued.InsurerTaxID)
		}

		var left []billing.SettlableSession
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			left, inner = repo.SettlableSessions(ctx, insurerID, monthFrom, monthTo)
			return inner
		}); err != nil {
			t.Fatalf("list after the issue: %v", err)
		}
		if len(left) != 0 {
			t.Fatalf("settlable after the issue = %d, want 0", len(left))
		}
	})

	t.Run("a session already in an active settlement is refused by a second one", func(t *testing.T) {
		again, err := billing.NewSettlement(billing.SettlementID(uuid.NewString()), tenant.ID(tenantA), users[tenantA], "IAM A", insurer, october,
			[]billing.SettlementItem{item(octoberSessions[0], sessions[octoberSessions[0]])}, now)
		if err != nil {
			t.Fatalf("domain issue: %v", err)
		}
		err = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, inner := repo.Issue(ctx, again)
			return inner
		})
		if !errors.Is(err, billing.ErrAppointmentAlreadySettled) {
			t.Fatalf("second settlement of a session: error = %v, want %v", err, billing.ErrAppointmentAlreadySettled)
		}
	})

	t.Run("the settlement reads back with its totals, and a missing one is not found", func(t *testing.T) {
		var got billing.Settlement
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			got, inner = repo.ByID(ctx, issued.ID)
			return inner
		}); err != nil {
			t.Fatalf("read the settlement: %v", err)
		}
		for _, row := range got.Items {
			if row.CopagoMinor != 150000 {
				t.Fatalf("copago of %s = %d, want 150000 kept from the issue", row.AppointmentID, row.CopagoMinor)
			}
		}
		totals := got.Totals()
		if len(got.Items) != 2 || len(totals) != 1 || totals[0].AmountMinor != 1600000 {
			t.Fatalf("settlement = %d items, totals %+v, want 2 items for 1600000 ARS", len(got.Items), totals)
		}

		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, inner := repo.ByID(ctx, billing.SettlementID(uuid.NewString()))
			return inner
		})
		if !errors.Is(err, billing.ErrNotFound) {
			t.Fatalf("missing settlement: error = %v, want %v", err, billing.ErrNotFound)
		}
	})

	t.Run("a void releases the sessions and the list shows the settlement of the mutual", func(t *testing.T) {
		var current billing.Settlement
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			current, inner = repo.ByID(ctx, issued.ID)
			return inner
		}); err != nil {
			t.Fatalf("read before the void: %v", err)
		}
		voided, err := current.Void(users[tenantA], "Lote mal armado", now.Add(time.Hour))
		if err != nil {
			t.Fatalf("domain void: %v", err)
		}
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			voided, inner = repo.Void(ctx, voided)
			return inner
		}); err != nil {
			t.Fatalf("void: %v", err)
		}
		if voided.Status != billing.StatusVoided || voided.VoidReason == nil || voided.Items[0].ReleasedAt == nil {
			t.Fatalf("voided = status %q, reason %v, released %v", voided.Status, voided.VoidReason, voided.Items[0].ReleasedAt)
		}

		var left []billing.SettlableSession
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			left, inner = repo.SettlableSessions(ctx, insurerID, monthFrom, monthTo)
			return inner
		}); err != nil {
			t.Fatalf("list after the void: %v", err)
		}
		if len(left) != 2 {
			t.Fatalf("settlable after the void = %d, want the two sessions again", len(left))
		}

		var listed []billing.Settlement
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			listed, inner = repo.List(ctx, port.SettlementFilter{InsurerID: insurerID, Limit: 20})
			return inner
		}); err != nil {
			t.Fatalf("list settlements: %v", err)
		}
		if len(listed) != 1 || listed[0].Status != billing.StatusVoided {
			t.Fatalf("list = %d settlements, want the one voided", len(listed))
		}
	})
}

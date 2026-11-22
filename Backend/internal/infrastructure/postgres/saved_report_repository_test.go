package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/report"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

type savedEnv struct {
	tariffEnv
	saved *postgres.SavedReportRepository
}

func newSavedEnv(t *testing.T) savedEnv {
	t.Helper()
	base := newTariffEnv(t)
	pool, err := pgxpool.New(context.Background(), base.appDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database as the application role: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return savedEnv{tariffEnv: base, saved: postgres.NewSavedReportRepository(pool)}
}

func (e savedEnv) build(t *testing.T, tenantID, author string, draft report.SavedDraft) report.SavedReport {
	t.Helper()
	subject, err := report.NewSaved(report.SavedID(uuid.NewString()), tenantID, author, draft, time.Now())
	if err != nil {
		t.Fatalf("cannot build the saved report: %v", err)
	}
	return subject
}

func (e savedEnv) create(tenantID string, subject report.SavedReport) error {
	return e.transactor.WithTenant(context.Background(), tenantID, func(ctx context.Context) error {
		return e.saved.Create(ctx, subject)
	})
}

func TestSavedReportRepository(t *testing.T) {
	env := newSavedEnv(t)
	ctx := context.Background()
	authorA := env.professionals[env.tenantA]
	authorB := env.professionals[env.tenantB]

	t.Run("a saved report reads back with its filters and the names the list shows", func(t *testing.T) {
		mutual := env.newMutual(t, env.tenantA, "OSDE Informes")
		draft := report.SavedDraft{
			Name: "Mutuales online", Period: "custom", From: "2026-11-01", To: "2026-11-30", GroupBy: "month",
			Filter: report.Filter{ProfessionalID: authorA, Coverage: "insurer", InsurerID: string(mutual.ID), Modality: "online"},
		}
		subject := env.build(t, env.tenantA, authorA, draft)
		if err := env.create(env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}

		err := env.transactor.WithTenant(ctx, env.tenantA, func(ctx context.Context) error {
			got, err := env.saved.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.Name != "Mutuales online" || got.Period != "custom" || got.From != "2026-11-01" || got.To != "2026-11-30" || got.GroupBy != "month" {
				t.Errorf("definition = %+v", got)
			}
			if got.Filter != draft.Filter {
				t.Errorf("filter = %+v, want %+v", got.Filter, draft.Filter)
			}
			items, err := env.saved.List(ctx)
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Report.ID != subject.ID {
					continue
				}
				if item.CreatedByName == "" || item.ProfessionalName == "" || item.InsurerName != "OSDE Informes" {
					t.Errorf("names = %q / %q / %q, want the author's, the professional's and the mutual's", item.CreatedByName, item.ProfessionalName, item.InsurerName)
				}
				return nil
			}
			t.Error("the saved report is missing from the list")
			return nil
		})
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
	})

	t.Run("a relative period keeps no days", func(t *testing.T) {
		subject := env.build(t, env.tenantA, authorA, report.SavedDraft{Name: "Este mes", Period: "this-month"})
		if err := env.create(env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}
		err := env.transactor.WithTenant(ctx, env.tenantA, func(ctx context.Context) error {
			got, err := env.saved.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.From != "" || got.To != "" || got.GroupBy != report.DimensionNone {
				t.Errorf("got %+v, want no days and no cut", got)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
	})

	t.Run("a name is unique in the practice ignoring case and outer spaces", func(t *testing.T) {
		first := env.build(t, env.tenantA, authorA, report.SavedDraft{Name: "Cierre", Period: "last-month"})
		if err := env.create(env.tenantA, first); err != nil {
			t.Fatalf("first: %v", err)
		}
		second := env.build(t, env.tenantA, authorA, report.SavedDraft{Name: " cIERRE ", Period: "this-year"})
		if err := env.create(env.tenantA, second); !errors.Is(err, report.ErrSavedNameTaken) {
			t.Fatalf("second = %v, want ErrSavedNameTaken", err)
		}
		other := env.build(t, env.tenantB, authorB, report.SavedDraft{Name: "Cierre", Period: "last-month"})
		if err := env.create(env.tenantB, other); err != nil {
			t.Fatalf("the same name in another practice must be fine: %v", err)
		}
	})

	t.Run("a professional of another practice is refused as a field error", func(t *testing.T) {
		subject := env.build(t, env.tenantA, authorA, report.SavedDraft{
			Name: "Ajeno", Period: "this-month", Filter: report.Filter{ProfessionalID: authorB},
		})
		var validation *report.ValidationError
		err := env.create(env.tenantA, subject)
		if !errors.As(err, &validation) {
			t.Fatalf("error = %v, want a validation error", err)
		}
	})

	t.Run("a mutual of another practice is refused as a field error", func(t *testing.T) {
		foreign := env.newMutual(t, env.tenantB, "OSDE Ajena Informes")
		subject := env.build(t, env.tenantA, authorA, report.SavedDraft{
			Name: "Mutual ajena", Period: "this-month", Filter: report.Filter{Coverage: "insurer", InsurerID: string(foreign.ID)},
		})
		var validation *report.ValidationError
		err := env.create(env.tenantA, subject)
		if !errors.As(err, &validation) {
			t.Fatalf("error = %v, want a validation error", err)
		}
	})

	t.Run("another practice neither reads, changes nor removes a saved report", func(t *testing.T) {
		subject := env.build(t, env.tenantA, authorA, report.SavedDraft{Name: "Solo de A", Period: "this-month"})
		if err := env.create(env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}
		err := env.transactor.WithTenant(ctx, env.tenantB, func(ctx context.Context) error {
			items, err := env.saved.List(ctx)
			if err != nil {
				return err
			}
			for _, item := range items {
				if item.Report.TenantID == env.tenantA {
					t.Errorf("practice B lists practice A's report %s", item.Report.ID)
				}
			}
			if _, err := env.saved.ByID(ctx, subject.ID); !errors.Is(err, report.ErrSavedNotFound) {
				t.Errorf("ByID across practices = %v, want ErrSavedNotFound", err)
			}
			if err := env.saved.Update(ctx, subject); !errors.Is(err, report.ErrSavedNotFound) {
				t.Errorf("Update across practices = %v, want ErrSavedNotFound", err)
			}
			if err := env.saved.Delete(ctx, subject.ID); !errors.Is(err, report.ErrSavedNotFound) {
				t.Errorf("Delete across practices = %v, want ErrSavedNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cross-practice access: %v", err)
		}
	})

	t.Run("a write outside a practice scope is refused before any statement", func(t *testing.T) {
		subject := env.build(t, env.tenantA, authorA, report.SavedDraft{Name: "Sin alcance", Period: "this-month"})
		if err := env.saved.Create(ctx, subject); !errors.Is(err, report.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want ErrTenantScopeMissing", err)
		}
	})

	t.Run("a change keeps the author and a removed report is not found again", func(t *testing.T) {
		subject := env.build(t, env.tenantA, authorA, report.SavedDraft{Name: "Para cambiar", Period: "this-month", GroupBy: "week"})
		if err := env.create(env.tenantA, subject); err != nil {
			t.Fatalf("create: %v", err)
		}
		revised, err := subject.Revise(report.SavedDraft{Name: "Cambiado", Period: "this-year", GroupBy: "year"}, time.Now().Add(time.Minute))
		if err != nil {
			t.Fatalf("revise: %v", err)
		}
		err = env.transactor.WithTenant(ctx, env.tenantA, func(ctx context.Context) error {
			if err := env.saved.Update(ctx, revised); err != nil {
				return err
			}
			got, err := env.saved.ByID(ctx, subject.ID)
			if err != nil {
				return err
			}
			if got.Name != "Cambiado" || got.Period != "this-year" || got.GroupBy != "year" || got.CreatedBy != authorA {
				t.Errorf("after the change = %+v", got)
			}
			if err := env.saved.Delete(ctx, subject.ID); err != nil {
				return err
			}
			return env.saved.Delete(ctx, subject.ID)
		})
		if !errors.Is(err, report.ErrSavedNotFound) {
			t.Fatalf("second delete = %v, want ErrSavedNotFound", err)
		}
	})
}

// TestSavedReportSchemaRefusesBrokenRows proves the table enforces the definition rules on its
// own, for a writer that bypasses the domain.
func TestSavedReportSchemaRefusesBrokenRows(t *testing.T) {
	env := newTariffEnv(t)
	ctx := context.Background()
	author := env.professionals[env.tenantA]

	conn, err := pgx.Connect(ctx, env.appDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	insert := func(name, period string, from, to *string, groupBy string, coverage *string, insurerID *string, modality *string) error {
		return withTenantSetting(ctx, conn, env.tenantA, func(ctx context.Context) error {
			_, err := conn.Exec(ctx, `
				INSERT INTO saved_reports (id, tenant_id, name, period, from_day, to_day, group_by, coverage, insurer_id, modality, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, now(), now())`,
				uuid.NewString(), env.tenantA, name, period, from, to, groupBy, coverage, insurerID, modality, author)
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
	day := func(value string) *string { return &value }

	t.Run("a custom period needs both days in order", func(t *testing.T) {
		expectCode(t, insert("c1", "custom", nil, nil, "none", nil, nil, nil), "23514")
		expectCode(t, insert("c2", "custom", day("2026-11-30"), day("2026-11-01"), "none", nil, nil, nil), "23514")
	})
	t.Run("a relative period carries no days", func(t *testing.T) {
		expectCode(t, insert("c3", "this-month", day("2026-11-01"), day("2026-11-30"), "none", nil, nil, nil), "23514")
	})
	t.Run("unknown period, cut, coverage and modality are refused", func(t *testing.T) {
		expectCode(t, insert("c4", "weekly", nil, nil, "none", nil, nil, nil), "23514")
		expectCode(t, insert("c5", "this-month", nil, nil, "color", nil, nil, nil), "23514")
		expectCode(t, insert("c6", "this-month", nil, nil, "none", day("mixed"), nil, nil), "23514")
		expectCode(t, insert("c7", "this-month", nil, nil, "none", nil, nil, day("hibrido")), "23514")
	})
	t.Run("a blank or overlong name is refused", func(t *testing.T) {
		expectCode(t, insert("   ", "this-month", nil, nil, "none", nil, nil, nil), "23514")
		long := make([]byte, 81)
		for i := range long {
			long[i] = 'x'
		}
		expectCode(t, insert(string(long), "this-month", nil, nil, "none", nil, nil, nil), "23514")
	})
	t.Run("a mutual cannot be paired with private patients", func(t *testing.T) {
		mutual := env.newMutual(t, env.tenantA, "OSDE Check")
		id := string(mutual.ID)
		expectCode(t, insert("c8", "this-month", nil, nil, "none", day("private"), &id, nil), "23514")
	})
	t.Run("a well-formed report is accepted", func(t *testing.T) {
		if err := insert("ok", "custom", day("2026-11-01"), day("2026-11-30"), "professional", day("insurer"), nil, day("online")); err != nil {
			t.Fatalf("a valid row: %v", err)
		}
	})
}

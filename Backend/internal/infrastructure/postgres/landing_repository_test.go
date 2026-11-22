package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/landing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// landingEnv builds a throwaway database with the real migrations, the landing adapters over
// the application role, and the two DSNs the schema checks need.
func landingEnv(t *testing.T) (env landingTestEnv) {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the landing tests")
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

	return landingTestEnv{
		transactor: postgres.NewTransactor(pool),
		landings:   postgres.NewLandingRepository(pool),
		domains:    postgres.NewTenantDomainRepository(pool),
		ownerDSN:   tempOwnerDSN,
		appDSN:     tempAppDSN,
		tenantA:    scratchTenantA,
		tenantB:    scratchTenantB,
	}
}

type landingTestEnv struct {
	transactor *postgres.Transactor
	landings   *postgres.LandingRepository
	domains    *postgres.TenantDomainRepository
	ownerDSN   string
	appDSN     string
	tenantA    string
	tenantB    string
}

func sampleLanding(t *testing.T, tenantID string) landing.Profile {
	t.Helper()
	subject, err := landing.New(tenant.ID(tenantID), landing.Draft{
		DisplayName: "Consultorio Sur",
		Tagline:     "Psicología clínica para adultos",
		About:       "Atendemos con turnos presenciales y online.",
		Address:     "Av. Siempre Viva 742",
		Phone:       "+54 11 4444-5555",
		Email:       "hola@sur.test",
		Schedule:    "Lunes a viernes, 9 a 18 h",
	}, time.Now())
	if err != nil {
		t.Fatalf("cannot build the landing: %v", err)
	}
	return subject
}

// sqlState returns the SQLSTATE of a Postgres error, or "" for any other error.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestLandingRepository(t *testing.T) {
	env := landingEnv(t)
	ctx := context.Background()

	save := func(tenantID string, subject landing.Profile) error {
		return env.transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			return env.landings.Save(ctx, subject)
		})
	}
	read := func(tenantID string) (landing.Profile, error) {
		var got landing.Profile
		err := env.transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
			var err error
			got, err = env.landings.Get(ctx)
			return err
		})
		return got, err
	}

	t.Run("a practice that has written nothing has no landing yet", func(t *testing.T) {
		if _, err := read(env.tenantA); !errors.Is(err, landing.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, landing.ErrNotFound)
		}
	})

	t.Run("a saved landing reads back with every field", func(t *testing.T) {
		subject := sampleLanding(t, env.tenantA)
		if err := save(env.tenantA, subject); err != nil {
			t.Fatalf("cannot save the landing: %v", err)
		}

		got, err := read(env.tenantA)
		if err != nil {
			t.Fatalf("cannot read the landing back: %v", err)
		}
		if got.DisplayName != subject.DisplayName || got.Tagline != subject.Tagline || got.About != subject.About ||
			got.Address != subject.Address || got.Phone != subject.Phone || got.Email != subject.Email ||
			got.Schedule != subject.Schedule {
			t.Fatalf("read %+v, want %+v", got.Draft, subject.Draft)
		}
		if string(got.TenantID) != env.tenantA {
			t.Fatalf("TenantID = %s, want %s", got.TenantID, env.tenantA)
		}
	})

	t.Run("a second save replaces the text and keeps one row", func(t *testing.T) {
		changed, err := sampleLanding(t, env.tenantA).Change(landing.Draft{DisplayName: "Consultorio Norte"}, time.Now())
		if err != nil {
			t.Fatalf("cannot change the landing: %v", err)
		}
		if err := save(env.tenantA, changed); err != nil {
			t.Fatalf("cannot save the change: %v", err)
		}

		got, err := read(env.tenantA)
		if err != nil {
			t.Fatalf("cannot read the landing back: %v", err)
		}
		if got.DisplayName != "Consultorio Norte" || got.Email != "" {
			t.Fatalf("read %+v, want the replaced text", got.Draft)
		}
	})

	t.Run("a practice reads only its own landing", func(t *testing.T) {
		if _, err := read(env.tenantB); !errors.Is(err, landing.ErrNotFound) {
			t.Fatalf("tenant B reads A's landing: error = %v, want %v", err, landing.ErrNotFound)
		}
	})

	t.Run("a write that names another practice is refused by the policy", func(t *testing.T) {
		err := save(env.tenantB, sampleLanding(t, env.tenantA))
		if sqlState(err) != "42501" {
			t.Fatalf("error = %v, want SQLSTATE 42501: the policy must refuse a row for another practice", err)
		}
	})

	t.Run("the landing is refused without a tenant scope", func(t *testing.T) {
		if _, err := env.landings.Get(ctx); !errors.Is(err, landing.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want %v", err, landing.ErrTenantScopeMissing)
		}
		if err := env.landings.Save(ctx, sampleLanding(t, env.tenantA)); !errors.Is(err, landing.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want %v", err, landing.ErrTenantScopeMissing)
		}
	})
}

func TestTenantDomainRepository(t *testing.T) {
	env := landingEnv(t)
	ctx := context.Background()

	owner, err := pgx.Connect(ctx, env.ownerDSN)
	if err != nil {
		t.Fatalf("cannot connect as the migrator: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close(ctx) })

	register := func(host, tenantID string) error {
		_, err := owner.Exec(ctx, `INSERT INTO tenant_domains (host, tenant_id, created_at) VALUES ($1, $2, now())`, host, tenantID)
		return err
	}

	if err := register("sur.paciente.test", env.tenantA); err != nil {
		t.Fatalf("cannot register the host: %v", err)
	}

	t.Run("a registered host answers its practice", func(t *testing.T) {
		got, err := env.domains.TenantIDByHost(ctx, "sur.paciente.test")
		if err != nil {
			t.Fatalf("TenantIDByHost: %v", err)
		}
		if string(got) != env.tenantA {
			t.Fatalf("TenantIDByHost = %s, want %s", got, env.tenantA)
		}
	})

	t.Run("an unknown host is tenant.ErrNotFound", func(t *testing.T) {
		if _, err := env.domains.TenantIDByHost(ctx, "nadie.test"); !errors.Is(err, tenant.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, tenant.ErrNotFound)
		}
	})

	t.Run("a host belongs to one practice only", func(t *testing.T) {
		err := register("sur.paciente.test", env.tenantB)
		if sqlState(err) != "23505" {
			t.Fatalf("error = %v, want SQLSTATE 23505", err)
		}
	})
}

// TestLandingSchemaRefusesWhatTheDomainRefuses: the CHECK constraints are the last line for a
// row written by any path, so they must refuse the same values the domain does.
func TestLandingSchemaRefusesWhatTheDomainRefuses(t *testing.T) {
	env := landingEnv(t)
	ctx := context.Background()

	app, err := pgx.Connect(ctx, env.appDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	t.Cleanup(func() { _ = app.Close(ctx) })

	insert := func(tenantID, displayName, tagline, email string) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO tenant_profiles (tenant_id, display_name, tagline, email, updated_at)
				VALUES ($1, $2, $3, $4, now())`, tenantID, displayName, tagline, email)
			return err
		})
	}

	t.Run("a blank name is refused", func(t *testing.T) {
		if err := insert(env.tenantA, "   ", "", ""); sqlState(err) != "23514" {
			t.Fatalf("error = %v, want SQLSTATE 23514", err)
		}
	})

	t.Run("a tagline over 160 characters is refused", func(t *testing.T) {
		if err := insert(env.tenantA, "Consultorio", strings.Repeat("ñ", 161), ""); sqlState(err) != "23514" {
			t.Fatalf("error = %v, want SQLSTATE 23514", err)
		}
	})

	t.Run("an email without an @ is refused", func(t *testing.T) {
		if err := insert(env.tenantA, "Consultorio", "", "sin-arroba"); sqlState(err) != "23514" {
			t.Fatalf("error = %v, want SQLSTATE 23514", err)
		}
	})

	t.Run("a valid landing is accepted", func(t *testing.T) {
		if err := insert(env.tenantA, "Consultorio", "Una línea", "hola@sur.test"); err != nil {
			t.Fatalf("a valid landing was refused: %v", err)
		}
	})

	owner, err := pgx.Connect(ctx, env.ownerDSN)
	if err != nil {
		t.Fatalf("cannot connect as the migrator: %v", err)
	}
	t.Cleanup(func() { _ = owner.Close(ctx) })

	t.Run("a host must be lowercase and have a dot", func(t *testing.T) {
		for _, host := range []string{"Sur.Paciente.test", "localhost"} {
			_, err := owner.Exec(ctx, `INSERT INTO tenant_domains (host, tenant_id, created_at) VALUES ($1, $2, now())`, host, env.tenantB)
			if sqlState(err) != "23514" {
				t.Fatalf("host %q: error = %v, want SQLSTATE 23514", host, err)
			}
		}
	})
}

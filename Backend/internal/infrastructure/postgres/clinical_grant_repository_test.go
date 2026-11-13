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
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// careFixture is a scratch database with the real migrations, two practices, and in the
// first one: a treating professional, another professional, and an assistant.
type careFixture struct {
	transactor *postgres.Transactor
	patients   *postgres.PatientRepository
	grants     *postgres.ClinicalGrantRepository
	catalog    *postgres.PatientCatalog
	treating   string
	grantee    string
	assistant  string
}

func careEnv(t *testing.T) careFixture {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the grant tests")
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

	fixture := careFixture{
		transactor: postgres.NewTransactor(pool),
		patients:   postgres.NewPatientRepository(pool),
		grants:     postgres.NewClinicalGrantRepository(pool),
		catalog:    postgres.NewPatientCatalog(pool),
	}
	fixture.treating, fixture.grantee, fixture.assistant = seedCareUsers(t, ctx, tempOwnerDSN, scratchTenantA)
	return fixture
}

// seedCareUsers inserts the three accounts as the owner, which bypasses row-level
// security the same way the other seeds in this package do.
func seedCareUsers(t *testing.T, ctx context.Context, ownerDSN, tenantID string) (treating, grantee, assistant string) {
	t.Helper()

	conn, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	insert := func(role, name string) string {
		id := uuid.NewString()
		if _, err := conn.Exec(ctx, `
			INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, 'hash', $4, $5, true, now(), now())`,
			id, tenantID, role+"-"+id[:8]+"@clinica.test", name, role); err != nil {
			t.Fatalf("cannot insert the %s user: %v", role, err)
		}
		return id
	}
	return insert("professional", "Profesional tratante"), insert("professional", "Profesional con acceso"), insert("assistant", "Recepción")
}

// treatedPatient builds a patient in the practice, treated by the given professional
// (empty means nobody), and stores it.
func (f careFixture) treatedPatient(t *testing.T, tenantID, name, treatingID string) patient.Patient {
	t.Helper()
	ctx := context.Background()

	subject := newPatient(t, f.catalog, tenantID, name, "")
	if treatingID != "" {
		subject = subject.WithTreating(treatingID)
	}
	if err := f.transactor.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		return f.patients.Create(ctx, subject)
	}); err != nil {
		t.Fatalf("cannot create the patient %q: %v", name, err)
	}
	return subject
}

func TestClinicalGrantLifecycle(t *testing.T) {
	fixture := careEnv(t)
	ctx := context.Background()
	tenantA := scratchTenantA

	subject := fixture.treatedPatient(t, tenantA, "Paciente con tratante", fixture.treating)
	granter := care.Viewer{UserID: fixture.treating, Role: iam.RoleProfessional}

	grant, err := care.NewGrant(uuid.NewString(), subject, granter, fixture.grantee, time.Now())
	if err != nil {
		t.Fatalf("the treating professional could not grant: %v", err)
	}

	t.Run("a grant persists and the grantee is reported as holding it", func(t *testing.T) {
		if err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return fixture.grants.Create(ctx, grant)
		}); err != nil {
			t.Fatalf("cannot create the grant: %v", err)
		}

		err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			live, err := fixture.grants.HasLiveGrant(ctx, string(subject.ID), fixture.grantee)
			if err != nil {
				return err
			}
			if !live {
				t.Error("HasLiveGrant = false right after the grant was created")
			}
			rows, err := fixture.grants.ListByPatient(ctx, string(subject.ID))
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].ProfessionalUserID != fixture.grantee || !rows[0].Live() {
				t.Errorf("ListByPatient = %+v, want the one live grant", rows)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot read the grant back: %v", err)
		}
	})

	t.Run("a second live grant to the same professional is refused", func(t *testing.T) {
		duplicate, err := care.NewGrant(uuid.NewString(), subject, granter, fixture.grantee, time.Now())
		if err != nil {
			t.Fatalf("cannot build the second grant: %v", err)
		}
		err = fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return fixture.grants.Create(ctx, duplicate)
		})
		if !errors.Is(err, care.ErrGrantAlreadyLive) {
			t.Fatalf("error = %v, want care.ErrGrantAlreadyLive from the partial unique index", err)
		}
	})

	t.Run("a revoked grant stops the read and can be granted again", func(t *testing.T) {
		err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			current, err := fixture.grants.ByID(ctx, grant.ID)
			if err != nil {
				return err
			}
			revoked, err := current.Revoke(granter, subject, time.Now())
			if err != nil {
				return err
			}
			return fixture.grants.Save(ctx, revoked)
		})
		if err != nil {
			t.Fatalf("cannot revoke the grant: %v", err)
		}

		err = fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			live, err := fixture.grants.HasLiveGrant(ctx, string(subject.ID), fixture.grantee)
			if err != nil {
				return err
			}
			if live {
				t.Error("HasLiveGrant = true after the revocation")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}

		again, err := care.NewGrant(uuid.NewString(), subject, granter, fixture.grantee, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return fixture.grants.Create(ctx, again)
		}); err != nil {
			t.Fatalf("a revoked grant blocked a new one: %v", err)
		}
	})

	t.Run("a grant is invisible to another practice", func(t *testing.T) {
		err := fixture.transactor.WithTenant(ctx, scratchTenantB, func(ctx context.Context) error {
			_, err := fixture.grants.ByID(ctx, grant.ID)
			return err
		})
		if !errors.Is(err, care.ErrGrantNotFound) {
			t.Fatalf("another practice read the grant: error = %v, want care.ErrGrantNotFound", err)
		}
	})

	t.Run("grant statements outside a tenant scope are refused, not run", func(t *testing.T) {
		if _, err := fixture.grants.HasLiveGrant(ctx, string(subject.ID), fixture.grantee); !errors.Is(err, care.ErrTenantScopeMissing) {
			t.Fatalf("error = %v, want care.ErrTenantScopeMissing", err)
		}
	})
}

// TestOwnOnlyListingFollowsTheTreatingAndGrants pins the professional's list: the
// patients it treats and the ones it holds a live grant on, and nothing else. The
// narrowing is in the query, so pagination counts only what the caller may see.
func TestOwnOnlyListingFollowsTheTreatingAndGrants(t *testing.T) {
	fixture := careEnv(t)
	ctx := context.Background()
	tenantA := scratchTenantA

	treated := fixture.treatedPatient(t, tenantA, "Tratado por el tratante", fixture.treating)
	// Nobody treats this one, so the treating professional sees it only through nothing:
	// it must stay out of its list, and out of the grantee's list too.
	fixture.treatedPatient(t, tenantA, "Tratado por otro", "")

	granter := care.Viewer{UserID: fixture.treating, Role: iam.RoleProfessional}
	grant, err := care.NewGrant(uuid.NewString(), treated, granter, fixture.grantee, time.Now())
	if err != nil {
		t.Fatalf("cannot build the grant: %v", err)
	}
	if err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		return fixture.grants.Create(ctx, grant)
	}); err != nil {
		t.Fatalf("cannot create the grant: %v", err)
	}

	list := func(t *testing.T, viewer string, ownOnly bool) map[string]bool {
		t.Helper()
		names := map[string]bool{}
		err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			rows, err := fixture.patients.List(ctx, port.PatientFilter{OwnOnly: ownOnly, ViewerID: viewer, Limit: 50})
			if err != nil {
				return err
			}
			for _, row := range rows {
				names[row.FullName] = true
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot list: %v", err)
		}
		return names
	}

	if got := list(t, fixture.treating, true); !got["Tratado por el tratante"] || got["Tratado por otro"] {
		t.Errorf("the treating professional sees %v, want only its own patient", got)
	}
	if got := list(t, fixture.grantee, true); !got["Tratado por el tratante"] || got["Tratado por otro"] {
		t.Errorf("the granted professional sees %v, want the granted patient and no other", got)
	}
	if got := list(t, fixture.assistant, false); len(got) != 2 {
		t.Errorf("the assistant sees %v, want the whole roster", got)
	}
}

// TestAssigningTheTreatingProfessionalPersists covers the reassignment: a patient
// without a treating professional gets one, and it reads back.
func TestAssigningTheTreatingProfessionalPersists(t *testing.T) {
	fixture := careEnv(t)
	ctx := context.Background()
	tenantA := scratchTenantA

	subject := fixture.treatedPatient(t, tenantA, "Sin tratante", "")
	if subject.TreatingProfessionalID != nil {
		t.Fatal("a new patient without a professional already has one")
	}

	assigned := subject.WithTreating(fixture.treating)
	assigned.UpdatedAt = time.Now()
	if err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		return fixture.patients.AssignTreating(ctx, assigned)
	}); err != nil {
		t.Fatalf("cannot assign the treating professional: %v", err)
	}

	err := fixture.transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		got, err := fixture.patients.ByID(ctx, subject.ID)
		if err != nil {
			return err
		}
		if got.TreatingProfessionalID == nil || *got.TreatingProfessionalID != fixture.treating {
			t.Errorf("TreatingProfessionalID = %v, want the assigned professional", got.TreatingProfessionalID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

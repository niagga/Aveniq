package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/niagga/aveniq/backend/internal/application/port"
	signupapp "github.com/niagga/aveniq/backend/internal/application/signup"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// signupTestClock fixes the instant so the trial end is asserted exactly.
type signupTestClock struct{ now time.Time }

func (c signupTestClock) Now() time.Time { return c.now }

type signupTestIDs struct{}

func (signupTestIDs) NewID() string { return uuid.NewString() }

// signupTestHasher stands in for argon2id: the storage contract does not depend on
// the hash function, and the real cost would dominate these tests.
type signupTestHasher struct{}

func (signupTestHasher) Hash(password string) (string, error) { return "hashed:" + password, nil }

// failingAuditLog makes the trail write fail, so the test can prove the practice and
// the owner written before it are rolled back together.
type failingAuditLog struct{}

func (failingAuditLog) Record(context.Context, port.AuditEvent) error {
	return errors.New("audit store unavailable")
}

var _ port.AuditLog = failingAuditLog{}

// countTrail reads the audit rows of one practice under its own tenant context. It
// goes through a transaction with app.tenant_id set, the same way the adapters do,
// because the audit table is row-level secured.
func countTrail(t *testing.T, pool interface {
	Begin(context.Context) (pgx.Tx, error)
}, tenantID, action string) int {
	t.Helper()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE action = $1", action).Scan(&count); err != nil {
		t.Fatalf("count trail: %v", err)
	}
	return count
}

func TestSignupWritesPracticeOwnerAndTrailTogether(t *testing.T) {
	pool, transactor, users, _, _ := iamEnv(t)
	tenants := postgres.NewTenantRepository(pool)
	audit := postgres.NewAuditRepository(pool)
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	uc := signupapp.NewUseCase(tenants, users, transactor, audit, signupTestClock{now: now}, signupTestIDs{}, signupTestHasher{})
	slug := "norte-" + uuid.NewString()[:8]

	result, err := uc.Execute(ctx, signupapp.Input{
		PracticeName: "Consultorio Norte",
		Slug:         slug,
		OwnerName:    "Ana Pérez",
		OwnerEmail:   "Ana@Norte.test",
		Password:     "clave-de-prueba-12",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	t.Run("the practice is stored in trial with its fourteen days", func(t *testing.T) {
		stored, err := tenants.BySlug(ctx, slug)
		if err != nil {
			t.Fatalf("BySlug: %v", err)
		}
		if stored.Status != tenant.StatusTrial {
			t.Fatalf("status = %q, want trial", stored.Status)
		}
		if want := now.Add(tenant.TrialDuration); !stored.TrialEndsAt.Equal(want) {
			t.Fatalf("trial_ends_at = %v, want %v", stored.TrialEndsAt, want)
		}
	})

	t.Run("the owner is stored under the new practice with the canonical email", func(t *testing.T) {
		var owner iam.User
		if err := transactor.WithTenant(ctx, result.TenantID, func(ctx context.Context) error {
			var err error
			owner, err = users.ByEmail(ctx, "ana@norte.test")
			return err
		}); err != nil {
			t.Fatalf("owner lookup: %v", err)
		}
		if owner.Role != iam.RoleOwner {
			t.Fatalf("role = %q, want owner", owner.Role)
		}
		if string(owner.PasswordHash) != "hashed:clave-de-prueba-12" {
			t.Fatalf("password hash = %q", owner.PasswordHash)
		}
	})

	t.Run("the trail names the registration in the new practice", func(t *testing.T) {
		if got := countTrail(t, pool, result.TenantID, "tenant.registered"); got != 1 {
			t.Fatalf("tenant.registered events = %d, want 1", got)
		}
	})
}

func TestSignupLeavesNothingBehindWhenTheTrailFails(t *testing.T) {
	pool, transactor, users, _, _ := iamEnv(t)
	tenants := postgres.NewTenantRepository(pool)
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	uc := signupapp.NewUseCase(tenants, users, transactor, failingAuditLog{}, signupTestClock{now: now}, signupTestIDs{}, signupTestHasher{})
	slug := "fallo-" + uuid.NewString()[:8]

	if _, err := uc.Execute(ctx, signupapp.Input{
		PracticeName: "Consultorio Fallido",
		Slug:         slug,
		OwnerName:    "Ana Pérez",
		OwnerEmail:   "ana@fallido.test",
		Password:     "clave-de-prueba-12",
	}); err == nil {
		t.Fatal("Execute succeeded although the trail could not be written")
	}

	if _, err := tenants.BySlug(ctx, slug); !errors.Is(err, tenant.ErrNotFound) {
		t.Fatalf("practice survived a failed registration: BySlug = %v, want ErrNotFound", err)
	}
}

func TestSignupRefusesATakenSlug(t *testing.T) {
	pool, transactor, users, _, _ := iamEnv(t)
	tenants := postgres.NewTenantRepository(pool)
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	uc := signupapp.NewUseCase(tenants, users, transactor, postgres.NewAuditRepository(pool), signupTestClock{now: now}, signupTestIDs{}, signupTestHasher{})

	// "iam-a" is the slug the IAM scratch database seeds for practice A.
	_, err := uc.Execute(ctx, signupapp.Input{
		PracticeName: "Otra",
		Slug:         "iam-a",
		OwnerName:    "Ana Pérez",
		OwnerEmail:   "ana@otra.test",
		Password:     "clave-de-prueba-12",
	})
	if !errors.Is(err, tenant.ErrSlugTaken) {
		t.Fatalf("Execute with a taken slug = %v, want ErrSlugTaken", err)
	}
}

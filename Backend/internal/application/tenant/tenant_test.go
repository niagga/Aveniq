package tenantapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// ─── Test doubles ────────────────────────────────────────────────────────────
// They are hand-written on purpose: the port is narrow enough that a mocking
// library would add indirection without adding clarity.

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fixedIDGenerator struct{ id string }

func (g fixedIDGenerator) NewID() string { return g.id }

type inMemoryTenantRepository struct {
	rows      map[string]tenant.Tenant
	createArg *tenant.Tenant
	createErr error
	listErr   error
	listLimit int32
}

func newInMemoryTenantRepository() *inMemoryTenantRepository {
	return &inMemoryTenantRepository{rows: map[string]tenant.Tenant{}}
}

func (r *inMemoryTenantRepository) Create(_ context.Context, subject tenant.Tenant) error {
	if r.createErr != nil {
		return r.createErr
	}
	if _, exists := r.rows[subject.Slug]; exists {
		return tenant.ErrSlugTaken
	}
	r.createArg = &subject
	r.rows[subject.Slug] = subject
	return nil
}

func (r *inMemoryTenantRepository) BySlug(_ context.Context, slug string) (tenant.Tenant, error) {
	subject, ok := r.rows[slug]
	if !ok {
		return tenant.Tenant{}, tenant.ErrNotFound
	}
	return subject, nil
}

// ByID exists because the port grew it for T19: an authenticated session resolves
// its practice from the token, never from the slug in the URL.
func (r *inMemoryTenantRepository) ByID(_ context.Context, id tenant.ID) (tenant.Tenant, error) {
	for _, subject := range r.rows {
		if subject.ID == id {
			return subject, nil
		}
	}
	return tenant.Tenant{}, tenant.ErrNotFound
}

func (r *inMemoryTenantRepository) List(_ context.Context, limit, _ int32) ([]tenant.Tenant, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	r.listLimit = limit

	rows := make([]tenant.Tenant, 0, len(r.rows))
	for _, subject := range r.rows {
		rows = append(rows, subject)
	}
	return rows, nil
}

func newUseCase(repo *inMemoryTenantRepository) *tenantapp.CreateUseCase {
	return tenantapp.NewCreateUseCase(
		repo,
		fixedClock{now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)},
		fixedIDGenerator{id: "0192f3a4-0000-7000-8000-00000000abcd"},
	)
}

// ─── Create ──────────────────────────────────────────────────────────────────

func TestCreateUseCaseExecute(t *testing.T) {
	t.Parallel()

	t.Run("persists a tenant built from generated identity and clock", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryTenantRepository()

		got, err := newUseCase(repo).Execute(context.Background(), tenantapp.CreateInput{
			Slug: "consultorio-sur",
			Name: "Consultorio Sur",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if repo.createArg == nil {
			t.Fatal("repository.Create was never called")
		}
		if repo.createArg.ID != "0192f3a4-0000-7000-8000-00000000abcd" {
			t.Errorf("persisted ID = %q, want the generated one", repo.createArg.ID)
		}
		if !repo.createArg.CreatedAt.Equal(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)) {
			t.Errorf("persisted CreatedAt = %v, want the clock value", repo.createArg.CreatedAt)
		}
		if got.Status != string(tenant.StatusTrial) {
			t.Errorf("Status = %q, want %q", got.Status, tenant.StatusTrial)
		}
		if got.Slug != "consultorio-sur" || got.Name != "Consultorio Sur" {
			t.Errorf("returned tenant = %+v, want the normalized input", got)
		}
	})

	t.Run("rejects invalid input without touching the repository", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryTenantRepository()

		_, err := newUseCase(repo).Execute(context.Background(), tenantapp.CreateInput{
			Slug: "not a slug!",
			Name: "Consultorio Sur",
		})

		var validationErr *tenant.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error = %v, want *tenant.ValidationError", err)
		}
		if validationErr.Field != "slug" {
			t.Errorf("Field = %q, want %q", validationErr.Field, "slug")
		}
		if repo.createArg != nil {
			t.Error("repository.Create must not be called when validation fails")
		}
	})

	t.Run("propagates the duplicate slug conflict", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryTenantRepository()
		repo.rows["consultorio-sur"] = tenant.Tenant{Slug: "consultorio-sur"}

		_, err := newUseCase(repo).Execute(context.Background(), tenantapp.CreateInput{
			Slug: "consultorio-sur",
			Name: "Otro Consultorio",
		})

		if !errors.Is(err, tenant.ErrSlugTaken) {
			t.Fatalf("error = %v, want %v", err, tenant.ErrSlugTaken)
		}
	})
}

// ─── List ────────────────────────────────────────────────────────────────────

func TestListUseCaseExecute(t *testing.T) {
	t.Parallel()

	t.Run("clamps the requested page size", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryTenantRepository()

		for _, slug := range []string{"alfa", "beta", "gamma"} {
			subject, err := tenant.New(tenant.ID("id-"+slug), slug, slug, time.Now())
			if err != nil {
				t.Fatalf("unexpected error building %s: %v", slug, err)
			}
			repo.rows[slug] = subject
		}

		got, err := tenantapp.NewListUseCase(repo).Execute(context.Background(), tenantapp.ListInput{Limit: 5000})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.listLimit != tenantapp.MaxPageSize {
			t.Errorf("repository limit = %d, want the clamped %d", repo.listLimit, tenantapp.MaxPageSize)
		}
		if len(got.Tenants) != 3 {
			t.Errorf("returned %d tenants, want 3", len(got.Tenants))
		}
	})

	t.Run("propagates repository failures", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryTenantRepository()
		repo.listErr = errors.New("connection reset")

		if _, err := tenantapp.NewListUseCase(repo).Execute(context.Background(), tenantapp.ListInput{}); err == nil {
			t.Fatal("expected the repository failure to propagate")
		}
	})
}

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

type inMemoryTenantRepository struct {
	rows      map[string]tenant.Tenant
	listErr   error
	listLimit int32
}

func newInMemoryTenantRepository() *inMemoryTenantRepository {
	return &inMemoryTenantRepository{rows: map[string]tenant.Tenant{}}
}

func (r *inMemoryTenantRepository) Create(_ context.Context, subject tenant.Tenant) error {
	if _, exists := r.rows[subject.Slug]; exists {
		return tenant.ErrSlugTaken
	}
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

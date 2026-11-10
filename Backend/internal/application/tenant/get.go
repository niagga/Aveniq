package tenantapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// GetBySlugInput identifies a tenant by its public slug.
type GetBySlugInput struct {
	Slug string
}

// GetBySlugUseCase resolves a tenant from the slug that appears in the subdomain.
type GetBySlugUseCase struct {
	repo port.TenantRepository
}

// NewGetBySlugUseCase wires the lookup use case.
func NewGetBySlugUseCase(repo port.TenantRepository) *GetBySlugUseCase {
	return &GetBySlugUseCase{repo: repo}
}

// Execute normalizes the slug before hitting the repository, so a trailing space
// or an uppercase slug resolves instead of producing a confusing 404.
func (uc *GetBySlugUseCase) Execute(ctx context.Context, in GetBySlugInput) (TenantView, error) {
	slug, err := tenant.NormalizeSlug(in.Slug)
	if err != nil {
		return TenantView{}, err
	}

	subject, err := uc.repo.BySlug(ctx, slug)
	if err != nil {
		return TenantView{}, err
	}

	return toView(subject), nil
}

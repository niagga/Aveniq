package tenantapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// GetByIDInput names the practice to resolve.
type GetByIDInput struct {
	ID string
}

// GetByIDUseCase resolves the practice an authenticated session belongs to.
//
// The identifier never comes from the request body or the URL: it comes from the
// verified access token, which is what makes it safe to pass straight to the
// repository.
type GetByIDUseCase struct {
	repo port.TenantRepository
}

// NewGetByIDUseCase wires the use case.
func NewGetByIDUseCase(repo port.TenantRepository) *GetByIDUseCase {
	return &GetByIDUseCase{repo: repo}
}

// Execute returns the practice or tenant.ErrNotFound.
func (uc *GetByIDUseCase) Execute(ctx context.Context, in GetByIDInput) (TenantView, error) {
	subject, err := uc.repo.ByID(ctx, tenant.ID(in.ID))
	if err != nil {
		return TenantView{}, err
	}
	return toView(subject), nil
}

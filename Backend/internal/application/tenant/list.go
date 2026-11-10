package tenantapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
)

// Pagination bounds. They live in the application layer because "how much may a
// caller ask for" is a policy decision, not a transport detail.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// ListInput requests one page of tenants.
type ListInput struct {
	Limit  int32
	Offset int32
}

// ListOutput is one page of tenants.
type ListOutput struct {
	Tenants []TenantView
}

// ListUseCase lists tenants visible to the caller.
type ListUseCase struct {
	repo port.TenantRepository
}

// NewListUseCase wires the list use case.
func NewListUseCase(repo port.TenantRepository) *ListUseCase {
	return &ListUseCase{repo: repo}
}

// Execute returns one page. It clamps the page size instead of rejecting it: an
// oversized request is a caller mistake that does not deserve a failure.
func (uc *ListUseCase) Execute(ctx context.Context, in ListInput) (ListOutput, error) {
	limit := in.Limit
	switch {
	case limit <= 0:
		limit = DefaultPageSize
	case limit > MaxPageSize:
		limit = MaxPageSize
	}

	offset := in.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := uc.repo.List(ctx, limit, offset)
	if err != nil {
		return ListOutput{}, err
	}

	views := make([]TenantView, 0, len(rows))
	for _, row := range rows {
		views = append(views, toView(row))
	}

	return ListOutput{Tenants: views}, nil
}

package patientapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// Pagination bounds. They live in the application layer because "how much may a
// caller ask for" is a policy decision, not a transport detail — and because the
// database answers an out-of-range limit with a raw SQLSTATE (L10: List(-1,-1)
// surfaced a raw 2201X), which is not an error a form can point at.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// ListInput requests one page of the practice's patients. A zero limit means the
// default page; a negative or oversized one is rejected instead of being passed
// through to the database.
type ListInput struct {
	// TenantID comes from the verified access token, never from the body.
	TenantID   string
	Search     string
	ActiveOnly bool
	Limit      int32
	Offset     int32
}

// ListUseCase lists patients visible to the caller.
type ListUseCase struct {
	repo  port.PatientRepository
	scope port.TenantScope
}

// NewListUseCase wires the use case.
func NewListUseCase(repo port.PatientRepository, scope port.TenantScope) *ListUseCase {
	return &ListUseCase{repo: repo, scope: scope}
}

// Execute returns one page of patients. Pagination is validated here, so the
// repository only ever sees a well-formed page.
func (uc *ListUseCase) Execute(ctx context.Context, in ListInput) ([]PatientView, error) {
	limit := in.Limit
	if limit == 0 {
		limit = DefaultPageSize
	}
	if limit < 0 || limit > MaxPageSize {
		return nil, &patient.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	if in.Offset < 0 {
		return nil, &patient.ValidationError{Field: "offset", Message: "must not be negative"}
	}

	var views []PatientView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		rows, err := uc.repo.List(ctx, port.PatientFilter{
			Search:     in.Search,
			ActiveOnly: in.ActiveOnly,
			Limit:      limit,
			Offset:     in.Offset,
		})
		if err != nil {
			return err
		}
		views = make([]PatientView, 0, len(rows))
		for _, row := range rows {
			views = append(views, toView(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

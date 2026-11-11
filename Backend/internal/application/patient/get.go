package patientapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// GetInput names the patient to resolve.
type GetInput struct {
	// TenantID comes from the verified access token, never from the body.
	TenantID string
	ID       string
}

// GetUseCase resolves one patient of the caller's practice.
//
// It opens the tenant scope even though it only reads: every PatientRepository
// method refuses to run outside WithTenant (L10), so a missing scope must be loud
// and never disguised as an empty result.
type GetUseCase struct {
	repo  port.PatientRepository
	scope port.TenantScope
}

// NewGetUseCase wires the use case.
func NewGetUseCase(repo port.PatientRepository, scope port.TenantScope) *GetUseCase {
	return &GetUseCase{repo: repo, scope: scope}
}

// Execute returns the patient or patient.ErrNotFound.
func (uc *GetUseCase) Execute(ctx context.Context, in GetInput) (PatientView, error) {
	var view PatientView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subject, err := uc.repo.ByID(ctx, patient.ID(in.ID))
		if err != nil {
			return err
		}
		view = toView(subject)
		return nil
	})
	if err != nil {
		return PatientView{}, err
	}
	return view, nil
}

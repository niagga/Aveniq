package patientapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// UpdateInput is the edited profile of an existing patient, plus the identity of
// the row to change.
type UpdateInput struct {
	ProfileInput
	// TenantID comes from the verified access token, never from the body.
	TenantID string
	ID       string
}

// UpdateUseCase edits the profile of an existing patient.
type UpdateUseCase struct {
	repo    port.PatientRepository
	catalog port.DocumentCatalog
	scope   port.TenantScope
	clock   port.Clock
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo port.PatientRepository, catalog port.DocumentCatalog, scope port.TenantScope, clock port.Clock) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, catalog: catalog, scope: scope, clock: clock}
}

// Execute loads the patient inside the tenant scope, applies the same
// normalization as creation (see normalizeProfile), and persists the edit.
// ChangeProfile keeps identity, creation time and the active flag untouched.
func (uc *UpdateUseCase) Execute(ctx context.Context, in UpdateInput) (PatientView, error) {
	draft, err := normalizeProfile(ctx, uc.catalog, in.TenantID, in.ProfileInput)
	if err != nil {
		return PatientView{}, err
	}

	var view PatientView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, patient.ID(in.ID))
		if err != nil {
			// A patient that does not exist is patient.ErrNotFound, untouched.
			return err
		}
		subject, err := current.ChangeProfile(draft, uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Update(ctx, subject); err != nil {
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

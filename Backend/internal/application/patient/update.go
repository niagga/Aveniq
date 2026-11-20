package patientapp

import (
	"context"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// changedInsurer reports whether the edit names a different mutual than the stored
// patient, or moves a private patient into one.
func changedInsurer(before, after patient.Patient) bool {
	if before.InsurerID == nil || after.InsurerID == nil {
		return before.InsurerID != after.InsurerID
	}
	return *before.InsurerID != *after.InsurerID
}

// UpdateInput is the edited profile of an existing patient, plus the identity of
// the row to change.
type UpdateInput struct {
	ProfileInput
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// UpdateUseCase edits the profile of an existing patient.
type UpdateUseCase struct {
	repo     port.PatientRepository
	insurers port.InsurerRepository
	catalog  port.DocumentCatalog
	scope    port.TenantScope
	clock    port.Clock
	access   *careapp.Access
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo port.PatientRepository, insurers port.InsurerRepository, catalog port.DocumentCatalog, scope port.TenantScope, clock port.Clock, access *careapp.Access) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, insurers: insurers, catalog: catalog, scope: scope, clock: clock, access: access}
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
		who := careapp.Viewer(in.ActorID, in.Role)
		resolved, err := uc.access.Resolve(ctx, who, in.ID)
		if err != nil {
			// A patient the caller may not see is patient.ErrNotFound, untouched.
			return err
		}
		if !care.EditsPatient(who, resolved.Patient) {
			return care.ErrForbidden
		}
		subject, err := resolved.Patient.ChangeProfile(draft, uc.clock.Now())
		if err != nil {
			return err
		}
		// Only a change of mutual is checked against the catalog. A patient whose mutual was
		// deactivated keeps it, and can still be edited, until someone changes it.
		if subject.CoverageKind == patient.CoverageInsurer && changedInsurer(resolved.Patient, subject) {
			if err := requireActiveInsurer(ctx, uc.insurers, *subject.InsurerID); err != nil {
				return err
			}
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

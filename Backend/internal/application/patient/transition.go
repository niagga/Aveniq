package patientapp

import (
	"context"
	"time"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// applyTransition loads a patient inside one tenant scope, applies a domain
// lifecycle change and persists it. Patients are never deleted (L18/L20): the
// only lifecycle moves are Deactivate and Activate, and both are exactly one
// load-modify-write inside exactly one WithTenant.
//
// The viewer must be allowed to edit the file (care.EditsPatient): a lifecycle change
// is an edit of the patient, so a professional deactivates only its own patients.
func applyTransition(
	ctx context.Context,
	repo port.PatientRepository,
	scope port.TenantScope,
	clock port.Clock,
	access *careapp.Access,
	who care.Viewer,
	tenantID, id string,
	change func(patient.Patient, time.Time) (patient.Patient, error),
) (PatientView, error) {
	var view PatientView
	err := scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		resolved, err := access.Resolve(ctx, who, id)
		if err != nil {
			// A patient the caller may not see is patient.ErrNotFound, untouched.
			return err
		}
		if !care.EditsPatient(who, resolved.Patient) {
			return care.ErrForbidden
		}
		subject, err := change(resolved.Patient, clock.Now())
		if err != nil {
			// A double transition is patient.ErrInvalidTransition, untouched.
			return err
		}
		if err := repo.Update(ctx, subject); err != nil {
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

package patientapp

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// applyTransition loads a patient inside one tenant scope, applies a domain
// lifecycle change and persists it. Patients are never deleted (L18/L20): the
// only lifecycle moves are Deactivate and Activate, and both are exactly one
// load-modify-write inside exactly one WithTenant.
func applyTransition(
	ctx context.Context,
	repo port.PatientRepository,
	scope port.TenantScope,
	clock port.Clock,
	tenantID, id string,
	change func(patient.Patient, time.Time) (patient.Patient, error),
) (PatientView, error) {
	var view PatientView
	err := scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		current, err := repo.ByID(ctx, patient.ID(id))
		if err != nil {
			// A patient that does not exist is patient.ErrNotFound, untouched.
			return err
		}
		subject, err := change(current, clock.Now())
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

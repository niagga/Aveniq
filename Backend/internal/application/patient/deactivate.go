package patientapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// DeactivateInput names the patient to take out of circulation.
type DeactivateInput struct {
	// TenantID comes from the verified access token, never from the body.
	TenantID string
	ID       string
}

// DeactivateUseCase deactivates a patient without losing anything: the row, its
// documents and its clinical history stay readable. There is no delete.
type DeactivateUseCase struct {
	repo  port.PatientRepository
	scope port.TenantScope
	clock port.Clock
}

// NewDeactivateUseCase wires the use case.
func NewDeactivateUseCase(repo port.PatientRepository, scope port.TenantScope, clock port.Clock) *DeactivateUseCase {
	return &DeactivateUseCase{repo: repo, scope: scope, clock: clock}
}

// Execute applies the domain transition or returns its error untouched.
func (uc *DeactivateUseCase) Execute(ctx context.Context, in DeactivateInput) (PatientView, error) {
	return applyTransition(ctx, uc.repo, uc.scope, uc.clock, in.TenantID, in.ID, patient.Patient.Deactivate)
}

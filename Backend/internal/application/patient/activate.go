package patientapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// ActivateInput names the patient to bring back.
type ActivateInput struct {
	// TenantID comes from the verified access token, never from the body.
	TenantID string
	ID       string
}

// ActivateUseCase reverses a deactivation.
type ActivateUseCase struct {
	repo  port.PatientRepository
	scope port.TenantScope
	clock port.Clock
}

// NewActivateUseCase wires the use case.
func NewActivateUseCase(repo port.PatientRepository, scope port.TenantScope, clock port.Clock) *ActivateUseCase {
	return &ActivateUseCase{repo: repo, scope: scope, clock: clock}
}

// Execute applies the domain transition or returns its error untouched.
func (uc *ActivateUseCase) Execute(ctx context.Context, in ActivateInput) (PatientView, error) {
	return applyTransition(ctx, uc.repo, uc.scope, uc.clock, in.TenantID, in.ID, patient.Patient.Activate)
}

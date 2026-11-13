package patientapp

import (
	"context"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// ActivateInput names the patient to bring back.
type ActivateInput struct {
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// ActivateUseCase reverses a deactivation.
type ActivateUseCase struct {
	repo   port.PatientRepository
	scope  port.TenantScope
	clock  port.Clock
	access *careapp.Access
}

// NewActivateUseCase wires the use case.
func NewActivateUseCase(repo port.PatientRepository, scope port.TenantScope, clock port.Clock, access *careapp.Access) *ActivateUseCase {
	return &ActivateUseCase{repo: repo, scope: scope, clock: clock, access: access}
}

// Execute applies the domain transition or returns its error untouched.
func (uc *ActivateUseCase) Execute(ctx context.Context, in ActivateInput) (PatientView, error) {
	return applyTransition(ctx, uc.repo, uc.scope, uc.clock, uc.access, careapp.Viewer(in.ActorID, in.Role),
		in.TenantID, in.ID, patient.Patient.Activate)
}

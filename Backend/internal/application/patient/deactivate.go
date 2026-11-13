package patientapp

import (
	"context"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// DeactivateInput names the patient to take out of circulation.
type DeactivateInput struct {
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// DeactivateUseCase deactivates a patient without losing anything: the row, its
// documents and its clinical history stay readable. There is no delete.
type DeactivateUseCase struct {
	repo   port.PatientRepository
	scope  port.TenantScope
	clock  port.Clock
	access *careapp.Access
}

// NewDeactivateUseCase wires the use case.
func NewDeactivateUseCase(repo port.PatientRepository, scope port.TenantScope, clock port.Clock, access *careapp.Access) *DeactivateUseCase {
	return &DeactivateUseCase{repo: repo, scope: scope, clock: clock, access: access}
}

// Execute applies the domain transition or returns its error untouched.
func (uc *DeactivateUseCase) Execute(ctx context.Context, in DeactivateInput) (PatientView, error) {
	return applyTransition(ctx, uc.repo, uc.scope, uc.clock, uc.access, careapp.Viewer(in.ActorID, in.Role),
		in.TenantID, in.ID, patient.Patient.Deactivate)
}

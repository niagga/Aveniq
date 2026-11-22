package patientapp

import (
	"context"
	"errors"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/client"
)

// GetInput names the patient to resolve.
type GetInput struct {
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// GetUseCase resolves one patient of the caller's practice.
//
// It opens the tenant scope even though it only reads: every PatientRepository
// method refuses to run outside WithTenant (L10), so a missing scope must be loud
// and never disguised as an empty result.
type GetUseCase struct {
	clients port.ClientRepository
	scope   port.TenantScope
	access  *careapp.Access
}

// NewGetUseCase wires the use case.
func NewGetUseCase(clients port.ClientRepository, scope port.TenantScope, access *careapp.Access) *GetUseCase {
	return &GetUseCase{clients: clients, scope: scope, access: access}
}

// Execute returns the patient the caller may see, or patient.ErrNotFound.
func (uc *GetUseCase) Execute(ctx context.Context, in GetInput) (PatientView, error) {
	var view PatientView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		resolved, err := uc.access.Resolve(ctx, careapp.Viewer(in.ActorID, in.Role), in.ID)
		if err != nil {
			return err
		}
		var payer *client.Client
		if resolved.Patient.ClientID != "" {
			found, err := uc.clients.ByID(ctx, client.ID(resolved.Patient.ClientID))
			if err != nil && !errors.Is(err, client.ErrNotFound) {
				return err
			}
			if err == nil {
				payer = &found
			}
		}
		view = toView(resolved.Patient, payer)
		return nil
	})
	if err != nil {
		return PatientView{}, err
	}
	return view, nil
}
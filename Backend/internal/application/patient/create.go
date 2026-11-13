package patientapp

import (
	"context"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// CreateInput is what the caller chooses for a new patient. Identity, timestamps
// and the active flag are absent because the system decides them.
type CreateInput struct {
	ProfileInput
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID string
	ActorID  string
	Role     string
}

// CreateUseCase registers a new patient.
type CreateUseCase struct {
	repo    port.PatientRepository
	catalog port.DocumentCatalog
	scope   port.TenantScope
	clock   port.Clock
	ids     port.IDGenerator
}

// NewCreateUseCase wires the use case through constructor injection: every
// dependency is explicit and replaceable, which is what keeps this layer testable
// without a database.
func NewCreateUseCase(repo port.PatientRepository, catalog port.DocumentCatalog, scope port.TenantScope, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{repo: repo, catalog: catalog, scope: scope, clock: clock, ids: ids}
}

// Execute creates the patient or returns a domain error. Normalization runs
// before the tenant transaction opens (see normalizeProfile); the write itself
// happens inside exactly one WithTenant, where row-level security reads
// app.tenant_id.
func (uc *CreateUseCase) Execute(ctx context.Context, in CreateInput) (PatientView, error) {
	draft, err := normalizeProfile(ctx, uc.catalog, in.TenantID, in.ProfileInput)
	if err != nil {
		return PatientView{}, err
	}

	var view PatientView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subject, err := patient.New(patient.ID(uc.ids.NewID()), tenant.ID(in.TenantID), draft, uc.clock.Now())
		if err != nil {
			return err
		}
		// A professional who registers a patient treats it: nobody else is set until an
		// owner or admin assigns one. The other roles register without a treating one.
		if care.BecomesTreatingOnCreate(careapp.Viewer(in.ActorID, in.Role)) {
			subject = subject.WithTreating(in.ActorID)
		}
		if err := uc.repo.Create(ctx, subject); err != nil {
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

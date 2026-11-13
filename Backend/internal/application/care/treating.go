package careapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// TreatingView is the treating professional of a patient as the API hands it out.
type TreatingView struct {
	PatientID      string
	ProfessionalID string
}

// AssignTreatingInput names the patient and the professional who will treat it.
type AssignTreatingInput struct {
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID       string
	ActorID        string
	Role           string
	PatientID      string
	ProfessionalID string
}

// AssignTreatingUseCase sets the treating professional of a patient.
//
// The permission patients:assign (owner and admin) is the gate; this use case adds the
// two facts the matrix cannot know: the patient must be one the caller may see, and the
// new treating professional must hold the professional role. The change is audited in
// the same transaction, because who treats a patient is part of the record.
type AssignTreatingUseCase struct {
	patients port.PatientRepository
	users    port.UserRepository
	scope    port.TenantScope
	audit    port.AuditLog
	clock    port.Clock
	access   *Access
}

// NewAssignTreatingUseCase wires the use case.
func NewAssignTreatingUseCase(
	patients port.PatientRepository,
	users port.UserRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	access *Access,
) *AssignTreatingUseCase {
	return &AssignTreatingUseCase{patients: patients, users: users, scope: scope, audit: audit, clock: clock, access: access}
}

// Execute assigns the treating professional or returns the refusal untouched.
func (uc *AssignTreatingUseCase) Execute(ctx context.Context, in AssignTreatingInput) (TreatingView, error) {
	who := Viewer(in.ActorID, in.Role)
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		resolved, err := uc.access.Resolve(ctx, who, in.PatientID)
		if err != nil {
			return err
		}

		professional, err := uc.users.ByID(ctx, iam.UserID(in.ProfessionalID))
		if err != nil {
			return err
		}
		if professional.Role != iam.RoleProfessional || !professional.IsActive {
			return care.ErrNotProfessional
		}

		subject := resolved.Patient.WithTreating(in.ProfessionalID)
		subject.UpdatedAt = uc.clock.Now()
		if err := uc.patients.AssignTreating(ctx, subject); err != nil {
			return err
		}
		return uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.ActorID,
			Action:   "patient.assign_treating",
			Entity:   "patient",
			EntityID: in.PatientID,
			Metadata: map[string]any{"professional_id": in.ProfessionalID},
		})
	})
	if err != nil {
		return TreatingView{}, err
	}
	return TreatingView{PatientID: in.PatientID, ProfessionalID: in.ProfessionalID}, nil
}

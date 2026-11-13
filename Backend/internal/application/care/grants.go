package careapp

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// GrantView is one clinical grant as the API hands it out.
type GrantView struct {
	ID             string
	PatientID      string
	ProfessionalID string
	GrantedBy      string
	GrantedAt      string
	RevokedAt      *string
}

func toGrantView(grant care.Grant) GrantView {
	view := GrantView{
		ID:             grant.ID,
		PatientID:      grant.PatientID,
		ProfessionalID: grant.ProfessionalUserID,
		GrantedBy:      grant.GrantedBy,
		GrantedAt:      grant.GrantedAt.UTC().Format(time.RFC3339),
	}
	if grant.RevokedAt != nil {
		revoked := grant.RevokedAt.UTC().Format(time.RFC3339)
		view.RevokedAt = &revoked
	}
	return view
}

// GrantInput names the patient and the professional who will read its clinical record.
type GrantInput struct {
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID       string
	ActorID        string
	Role           string
	PatientID      string
	ProfessionalID string
}

// GrantClinicalUseCase gives another professional read access to a patient's file and
// clinical record. Only the treating professional gives a grant (care.GrantsClinical).
type GrantClinicalUseCase struct {
	grants port.ClinicalGrantRepository
	users  port.UserRepository
	scope  port.TenantScope
	audit  port.AuditLog
	clock  port.Clock
	ids    port.IDGenerator
	access *Access
}

// NewGrantClinicalUseCase wires the use case.
func NewGrantClinicalUseCase(
	grants port.ClinicalGrantRepository,
	users port.UserRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	ids port.IDGenerator,
	access *Access,
) *GrantClinicalUseCase {
	return &GrantClinicalUseCase{grants: grants, users: users, scope: scope, audit: audit, clock: clock, ids: ids, access: access}
}

// Execute creates the grant. A second live grant to the same professional is
// care.ErrGrantAlreadyLive, from the database's partial unique index.
func (uc *GrantClinicalUseCase) Execute(ctx context.Context, in GrantInput) (GrantView, error) {
	who := Viewer(in.ActorID, in.Role)

	var view GrantView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		resolved, err := uc.access.Resolve(ctx, who, in.PatientID)
		if err != nil {
			return err
		}

		grant, err := care.NewGrant(uc.ids.NewID(), resolved.Patient, who, in.ProfessionalID, uc.clock.Now())
		if err != nil {
			return err
		}

		grantee, err := uc.users.ByID(ctx, iam.UserID(in.ProfessionalID))
		if err != nil {
			return err
		}
		if grantee.Role != iam.RoleProfessional || !grantee.IsActive {
			return care.ErrNotProfessional
		}

		if err := uc.grants.Create(ctx, grant); err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, port.AuditEvent{
			TenantID: in.TenantID,
			ActorID:  in.ActorID,
			Action:   "clinical.grant",
			Entity:   "clinical_grant",
			EntityID: grant.ID,
			Metadata: map[string]any{"patient_id": in.PatientID, "professional_id": in.ProfessionalID},
		}); err != nil {
			return err
		}
		view = toGrantView(grant)
		return nil
	})
	if err != nil {
		return GrantView{}, err
	}
	return view, nil
}

// ListGrantsInput names the patient whose grants are listed.
type ListGrantsInput struct {
	TenantID  string
	ActorID   string
	Role      string
	PatientID string
}

// ListGrantsUseCase lists the grants over a patient, live and revoked. Only the
// treating professional and the owner manage them, so only they see the list.
type ListGrantsUseCase struct {
	grants port.ClinicalGrantRepository
	scope  port.TenantScope
	access *Access
}

// NewListGrantsUseCase wires the use case.
func NewListGrantsUseCase(grants port.ClinicalGrantRepository, scope port.TenantScope, access *Access) *ListGrantsUseCase {
	return &ListGrantsUseCase{grants: grants, scope: scope, access: access}
}

// Execute returns the grants or the refusal.
func (uc *ListGrantsUseCase) Execute(ctx context.Context, in ListGrantsInput) ([]GrantView, error) {
	who := Viewer(in.ActorID, in.Role)

	var views []GrantView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		resolved, err := uc.access.Resolve(ctx, who, in.PatientID)
		if err != nil {
			return err
		}
		if !care.ManagesGrants(who, resolved.Patient) {
			return care.ErrForbidden
		}

		rows, err := uc.grants.ListByPatient(ctx, in.PatientID)
		if err != nil {
			return err
		}
		views = make([]GrantView, 0, len(rows))
		for _, row := range rows {
			views = append(views, toGrantView(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// RevokeGrantInput names the grant to withdraw.
type RevokeGrantInput struct {
	TenantID string
	ActorID  string
	Role     string
	GrantID  string
}

// RevokeGrantUseCase withdraws a grant. Revoking a grant that is already revoked
// returns it unchanged, so the call is safe to repeat.
type RevokeGrantUseCase struct {
	grants port.ClinicalGrantRepository
	scope  port.TenantScope
	audit  port.AuditLog
	clock  port.Clock
	access *Access
}

// NewRevokeGrantUseCase wires the use case.
func NewRevokeGrantUseCase(grants port.ClinicalGrantRepository, scope port.TenantScope, audit port.AuditLog, clock port.Clock, access *Access) *RevokeGrantUseCase {
	return &RevokeGrantUseCase{grants: grants, scope: scope, audit: audit, clock: clock, access: access}
}

// Execute withdraws the grant or returns the refusal.
func (uc *RevokeGrantUseCase) Execute(ctx context.Context, in RevokeGrantInput) (GrantView, error) {
	who := Viewer(in.ActorID, in.Role)

	var view GrantView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.grants.ByID(ctx, in.GrantID)
		if err != nil {
			return err
		}
		resolved, err := uc.access.Resolve(ctx, who, current.PatientID)
		if err != nil {
			return err
		}

		revoked, err := current.Revoke(who, resolved.Patient, uc.clock.Now())
		if err != nil {
			return err
		}
		if current.Live() {
			if err := uc.grants.Save(ctx, revoked); err != nil {
				return err
			}
			if err := uc.audit.Record(ctx, port.AuditEvent{
				TenantID: in.TenantID,
				ActorID:  in.ActorID,
				Action:   "clinical.revoke",
				Entity:   "clinical_grant",
				EntityID: revoked.ID,
				Metadata: map[string]any{"patient_id": revoked.PatientID, "professional_id": revoked.ProfessionalUserID},
			}); err != nil {
				return err
			}
		}
		view = toGrantView(revoked)
		return nil
	})
	if err != nil {
		return GrantView{}, err
	}
	return view, nil
}

// Package insurerapp holds the mutual catalog use cases: the application layer between
// the HTTP handlers and the insurer aggregate. Writes are gated by the permission matrix
// at the transport (tenant:settings: owner and admin); reads by patients:read, because
// the patient forms need the catalog to choose a mutual.
package insurerapp

import (
	"context"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// timeLayout matches the rest of the API: RFC 3339, always UTC.
const timeLayout = time.RFC3339

// ProfileInput is what a caller chooses for a mutual. It is shared by create and edit,
// the same way the patient profile is.
type ProfileInput struct {
	Name string
	// TaxID is the CUIT as typed. Empty means absent.
	TaxID string
}

// InsurerView is the read model the application exposes to delivery: flat and
// string-based, so transports never see domain types.
type InsurerView struct {
	ID        string
	TenantID  string
	Name      string
	TaxID     string // digits only, empty when absent
	IsActive  bool
	CreatedAt string
	UpdatedAt string
}

func toView(subject insurer.Insurer) InsurerView {
	return InsurerView{
		ID:        string(subject.ID),
		TenantID:  string(subject.TenantID),
		Name:      subject.Name,
		TaxID:     derefString(subject.TaxID),
		IsActive:  subject.IsActive,
		CreatedAt: subject.CreatedAt.UTC().Format(timeLayout),
		UpdatedAt: subject.UpdatedAt.UTC().Format(timeLayout),
	}
}

// draftFrom turns the typed input into the aggregate's draft. An empty tax id is absent,
// not a blank string the domain would have to reject.
func draftFrom(in ProfileInput) insurer.Draft {
	draft := insurer.Draft{Name: strings.TrimSpace(in.Name)}
	if taxID := strings.TrimSpace(in.TaxID); taxID != "" {
		draft.TaxID = &taxID
	}
	return draft
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ListInput selects the catalog to read.
type ListInput struct {
	TenantID   string
	ActiveOnly bool
}

// ListUseCase lists the mutuals of the practice.
type ListUseCase struct {
	repo  port.InsurerRepository
	scope port.TenantScope
}

// NewListUseCase wires the use case.
func NewListUseCase(repo port.InsurerRepository, scope port.TenantScope) *ListUseCase {
	return &ListUseCase{repo: repo, scope: scope}
}

// Execute returns the catalog ordered by name.
func (uc *ListUseCase) Execute(ctx context.Context, in ListInput) ([]InsurerView, error) {
	var views []InsurerView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subjects, err := uc.repo.List(ctx, in.ActiveOnly)
		if err != nil {
			return err
		}
		views = make([]InsurerView, 0, len(subjects))
		for _, subject := range subjects {
			views = append(views, toView(subject))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// CreateInput is a new mutual for a practice.
type CreateInput struct {
	ProfileInput
	// TenantID comes from the verified access token, never from the body.
	TenantID string
	ActorID  string
	Role     string
}

// CreateUseCase adds a mutual to the catalog.
type CreateUseCase struct {
	repo  port.InsurerRepository
	scope port.TenantScope
	clock port.Clock
	ids   port.IDGenerator
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo port.InsurerRepository, scope port.TenantScope, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{repo: repo, scope: scope, clock: clock, ids: ids}
}

// Execute validates the mutual in the domain and inserts it inside the tenant scope.
func (uc *CreateUseCase) Execute(ctx context.Context, in CreateInput) (InsurerView, error) {
	var view InsurerView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subject, err := insurer.New(insurer.ID(uc.ids.NewID()), tenant.ID(in.TenantID), draftFrom(in.ProfileInput), uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Create(ctx, subject); err != nil {
			return err
		}
		view = toView(subject)
		return nil
	})
	if err != nil {
		return InsurerView{}, err
	}
	return view, nil
}

// UpdateInput edits a mutual's name and tax id.
type UpdateInput struct {
	ProfileInput
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// UpdateUseCase edits an existing mutual. Its active flag is changed by the lifecycle
// use cases, never here.
type UpdateUseCase struct {
	repo  port.InsurerRepository
	scope port.TenantScope
	clock port.Clock
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo port.InsurerRepository, scope port.TenantScope, clock port.Clock) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, scope: scope, clock: clock}
}

// Execute loads the mutual, applies the edit, and persists it.
func (uc *UpdateUseCase) Execute(ctx context.Context, in UpdateInput) (InsurerView, error) {
	var view InsurerView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, insurer.ID(in.ID))
		if err != nil {
			return err
		}
		updated, err := current.ChangeDetails(draftFrom(in.ProfileInput), uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Update(ctx, updated); err != nil {
			return err
		}
		view = toView(updated)
		return nil
	})
	if err != nil {
		return InsurerView{}, err
	}
	return view, nil
}

// LifecycleInput identifies the mutual to deactivate or reactivate.
type LifecycleInput struct {
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// LifecycleUseCase covers Deactivate and Activate, which differ only in the transition.
type LifecycleUseCase struct {
	repo       port.InsurerRepository
	scope      port.TenantScope
	clock      port.Clock
	transition func(insurer.Insurer, time.Time) (insurer.Insurer, error)
}

// NewDeactivateUseCase builds the use case that stops new coverage on a mutual.
func NewDeactivateUseCase(repo port.InsurerRepository, scope port.TenantScope, clock port.Clock) *LifecycleUseCase {
	return &LifecycleUseCase{repo: repo, scope: scope, clock: clock, transition: insurer.Insurer.Deactivate}
}

// NewActivateUseCase builds the use case that makes a mutual available again.
func NewActivateUseCase(repo port.InsurerRepository, scope port.TenantScope, clock port.Clock) *LifecycleUseCase {
	return &LifecycleUseCase{repo: repo, scope: scope, clock: clock, transition: insurer.Insurer.Activate}
}

// Execute applies the transition and persists the result.
func (uc *LifecycleUseCase) Execute(ctx context.Context, in LifecycleInput) (InsurerView, error) {
	var view InsurerView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, insurer.ID(in.ID))
		if err != nil {
			return err
		}
		next, err := uc.transition(current, uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Update(ctx, next); err != nil {
			return err
		}
		view = toView(next)
		return nil
	})
	if err != nil {
		return InsurerView{}, err
	}
	return view, nil
}

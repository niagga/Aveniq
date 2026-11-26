// Package clientapp holds the billing-client use cases (E8/T71).
package clientapp

import (
	"context"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

const timeLayout = time.RFC3339

// ProfileInput is the fiscal profile of a billing client.
type ProfileInput struct {
	DisplayName string
	LegalName   string
	Country     string

	TaxIDKind   string
	TaxIDNumber string

	FiscalCondition string
	FiscalAddress   string

	BillingCurrency string
}

// ClientView is the read model exposed to HTTP.
type ClientView struct {
	ID        string
	TenantID  string
	DisplayName string
	LegalName string
	Country   string

	TaxIDKind   string
	TaxIDNumber string

	FiscalCondition string
	FiscalAddress   string

	BillingCurrency string

	IsActive  bool
	CreatedAt string
	UpdatedAt string
}

func toView(subject client.Client) ClientView {
	view := ClientView{
		ID:              string(subject.ID),
		TenantID:        string(subject.TenantID),
		DisplayName:     subject.DisplayName,
		Country:         subject.Country,
		FiscalCondition: derefString(subject.FiscalCondition),
		FiscalAddress:   derefString(subject.FiscalAddress),
		BillingCurrency: string(subject.BillingCurrency),
		IsActive:        subject.IsActive,
		CreatedAt:       subject.CreatedAt.UTC().Format(timeLayout),
		UpdatedAt:       subject.UpdatedAt.UTC().Format(timeLayout),
	}
	if subject.LegalName != nil {
		view.LegalName = *subject.LegalName
	}
	if subject.TaxID != nil {
		view.TaxIDKind = subject.TaxID.Kind
		view.TaxIDNumber = subject.TaxID.Number
	}
	return view
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// DraftFromProfile normalizes fiscal client input against the country catalog.
func DraftFromProfile(ctx context.Context, catalog port.DocumentCatalog, tenantID string, in ProfileInput) (client.Draft, error) {
	return draftFrom(ctx, catalog, tenantID, in)
}

func draftFrom(ctx context.Context, catalog port.DocumentCatalog, tenantID string, in ProfileInput) (client.Draft, error) {
	country := strings.TrimSpace(in.Country)
	if country == "" {
		resolved, err := catalog.TenantCountry(ctx, tenantID)
		if err != nil {
			return client.Draft{}, err
		}
		country = resolved
	}

	var taxID *patient.TaxID
	if in.TaxIDKind == "" && in.TaxIDNumber == "" {
		taxID = nil
	} else if in.TaxIDKind == "" {
		return client.Draft{}, &patient.ValidationError{Field: "tax_id_kind", Message: "is required when a tax id number is given"}
	} else if strings.TrimSpace(in.TaxIDNumber) == "" {
		return client.Draft{}, &patient.ValidationError{Field: "tax_id_number", Message: "must not be blank"}
	} else {
		kind, err := catalog.TaxIDKind(ctx, country, in.TaxIDKind)
		if err != nil {
			return client.Draft{}, err
		}
		normalized, err := patient.NormalizeTaxID(kind, in.TaxIDNumber)
		if err != nil {
			return client.Draft{}, err
		}
		taxID = &normalized
	}

	fiscalCondition := strings.TrimSpace(in.FiscalCondition)
	if fiscalCondition != "" {
		if _, err := catalog.FiscalCondition(ctx, country, fiscalCondition); err != nil {
			return client.Draft{}, err
		}
	}

	currency := patient.Currency(strings.TrimSpace(in.BillingCurrency))
	if currency == "" {
		return client.Draft{}, &patient.ValidationError{Field: "billing_currency", Message: "is required"}
	}

	draft := client.Draft{
		DisplayName:     strings.TrimSpace(in.DisplayName),
		Country:         country,
		TaxID:           taxID,
		BillingCurrency: currency,
	}
	if legal := strings.TrimSpace(in.LegalName); legal != "" {
		draft.LegalName = &legal
	}
	if fiscalCondition != "" {
		draft.FiscalCondition = &fiscalCondition
	}
	if addr := strings.TrimSpace(in.FiscalAddress); addr != "" {
		draft.FiscalAddress = &addr
	}
	return draft, nil
}

type ListInput struct {
	TenantID   string
	ActiveOnly bool
}

type ListUseCase struct {
	repo  port.ClientRepository
	scope port.TenantScope
}

func NewListUseCase(repo port.ClientRepository, scope port.TenantScope) *ListUseCase {
	return &ListUseCase{repo: repo, scope: scope}
}

func (uc *ListUseCase) Execute(ctx context.Context, in ListInput) ([]ClientView, error) {
	var views []ClientView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subjects, err := uc.repo.List(ctx, in.ActiveOnly)
		if err != nil {
			return err
		}
		views = make([]ClientView, 0, len(subjects))
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

type GetInput struct {
	TenantID string
	ID       string
}

type GetUseCase struct {
	repo  port.ClientRepository
	scope port.TenantScope
}

func NewGetUseCase(repo port.ClientRepository, scope port.TenantScope) *GetUseCase {
	return &GetUseCase{repo: repo, scope: scope}
}

func (uc *GetUseCase) Execute(ctx context.Context, in GetInput) (ClientView, error) {
	var view ClientView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subject, err := uc.repo.ByID(ctx, client.ID(in.ID))
		if err != nil {
			return err
		}
		view = toView(subject)
		return nil
	})
	if err != nil {
		return ClientView{}, err
	}
	return view, nil
}

type CreateInput struct {
	ProfileInput
	TenantID string
	ActorID  string
	Role     string
}

type CreateUseCase struct {
	repo    port.ClientRepository
	catalog port.DocumentCatalog
	scope   port.TenantScope
	clock   port.Clock
	ids     port.IDGenerator
}

func NewCreateUseCase(repo port.ClientRepository, catalog port.DocumentCatalog, scope port.TenantScope, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{repo: repo, catalog: catalog, scope: scope, clock: clock, ids: ids}
}

func (uc *CreateUseCase) Execute(ctx context.Context, in CreateInput) (ClientView, error) {
	draft, err := draftFrom(ctx, uc.catalog, in.TenantID, in.ProfileInput)
	if err != nil {
		return ClientView{}, err
	}

	var view ClientView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subject, err := client.New(client.ID(uc.ids.NewID()), tenant.ID(in.TenantID), draft, uc.clock.Now())
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
		return ClientView{}, err
	}
	return view, nil
}

type UpdateInput struct {
	ProfileInput
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

type UpdateUseCase struct {
	repo    port.ClientRepository
	catalog port.DocumentCatalog
	scope   port.TenantScope
	clock   port.Clock
}

func NewUpdateUseCase(repo port.ClientRepository, catalog port.DocumentCatalog, scope port.TenantScope, clock port.Clock) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, catalog: catalog, scope: scope, clock: clock}
}

func (uc *UpdateUseCase) Execute(ctx context.Context, in UpdateInput) (ClientView, error) {
	draft, err := draftFrom(ctx, uc.catalog, in.TenantID, in.ProfileInput)
	if err != nil {
		return ClientView{}, err
	}

	var view ClientView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, client.ID(in.ID))
		if err != nil {
			return err
		}
		updated, err := current.ChangeProfile(draft, uc.clock.Now())
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
		return ClientView{}, err
	}
	return view, nil
}

type LifecycleInput struct {
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

type LifecycleUseCase struct {
	repo       port.ClientRepository
	scope      port.TenantScope
	clock      port.Clock
	transition func(client.Client, time.Time) (client.Client, error)
}

func NewDeactivateUseCase(repo port.ClientRepository, scope port.TenantScope, clock port.Clock) *LifecycleUseCase {
	return &LifecycleUseCase{repo: repo, scope: scope, clock: clock, transition: client.Client.Deactivate}
}

func NewActivateUseCase(repo port.ClientRepository, scope port.TenantScope, clock port.Clock) *LifecycleUseCase {
	return &LifecycleUseCase{repo: repo, scope: scope, clock: clock, transition: client.Client.Activate}
}

func (uc *LifecycleUseCase) Execute(ctx context.Context, in LifecycleInput) (ClientView, error) {
	var view ClientView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, client.ID(in.ID))
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
		return ClientView{}, err
	}
	return view, nil
}
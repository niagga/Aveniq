// Package pricingapp holds the fee schedule (tarifario) use cases: the application layer
// between the HTTP handlers and the tariff aggregate (L61). Writes are gated by the
// permission matrix at the transport (tenant:settings: owner and admin). Reads are gated
// by billing:read, because the accountant reads the schedule and it is the value the
// vouchers start from (T54).
package pricingapp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// dayLayout is how a start day travels: a civil date, no clock, no zone.
const dayLayout = "2006-01-02"

// TariffView is the read model the application exposes to delivery: flat and string-based,
// so transports never see domain types. TotalMinor is what the session is worth.
type TariffView struct {
	ID                 string
	TenantID           string
	ProfessionalUserID string
	ProfessionalName   string
	Coverage           string
	InsurerID          string // empty for a particular tariff
	InsurerName        string // empty for a particular tariff
	Modality           string
	Currency           string
	ValidFrom          string
	InsurerMinor       int64
	PatientMinor       int64
	TotalMinor         int64
	CreatedAt          string
	UpdatedAt          string
}

func toView(subject pricing.Tariff, professionalName, insurerName string) TariffView {
	return TariffView{
		ID:                 string(subject.ID),
		TenantID:           string(subject.TenantID),
		ProfessionalUserID: subject.ProfessionalUserID,
		ProfessionalName:   professionalName,
		Coverage:           string(subject.Coverage),
		InsurerID:          derefString(subject.InsurerID),
		InsurerName:        insurerName,
		Modality:           string(subject.Modality),
		Currency:           string(subject.Currency),
		ValidFrom:          subject.ValidFrom.Format(dayLayout),
		InsurerMinor:       subject.InsurerMinor,
		PatientMinor:       subject.PatientMinor,
		TotalMinor:         subject.TotalMinor(),
		CreatedAt:          subject.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:          subject.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ListInput selects the schedule of a practice.
type ListInput struct {
	TenantID string
}

// ListUseCase lists the tariffs of the practice.
type ListUseCase struct {
	repo  port.TariffRepository
	scope port.TenantScope
}

// NewListUseCase wires the use case.
func NewListUseCase(repo port.TariffRepository, scope port.TenantScope) *ListUseCase {
	return &ListUseCase{repo: repo, scope: scope}
}

// Execute returns the schedule, already ordered for reading.
func (uc *ListUseCase) Execute(ctx context.Context, in ListInput) ([]TariffView, error) {
	var views []TariffView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		items, err := uc.repo.List(ctx)
		if err != nil {
			return err
		}
		views = make([]TariffView, 0, len(items))
		for _, item := range items {
			views = append(views, toView(item.Tariff, item.ProfessionalName, item.InsurerName))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// CreateInput is a new price. The amounts are minor units of Currency: for a particular
// price InsurerMinor is zero and PatientMinor is the whole fee.
type CreateInput struct {
	TenantID           string
	ActorID            string
	Role               string
	ProfessionalUserID string
	Coverage           string
	// InsurerID is the mutual of a mutual price. Empty for a particular one.
	InsurerID    string
	Modality     string
	Currency     string
	ValidFrom    time.Time
	InsurerMinor int64
	PatientMinor int64
}

// CreateUseCase adds a price to the schedule.
type CreateUseCase struct {
	repo     port.TariffRepository
	insurers port.InsurerRepository
	scope    port.TenantScope
	clock    port.Clock
	ids      port.IDGenerator
}

// NewCreateUseCase wires the use case. The insurer repository is there to refuse a mutual
// that is not in the catalog or has been deactivated.
func NewCreateUseCase(repo port.TariffRepository, insurers port.InsurerRepository, scope port.TenantScope, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{repo: repo, insurers: insurers, scope: scope, clock: clock, ids: ids}
}

// Execute validates the price in the domain, checks its mutual, and inserts it inside the
// tenant scope.
func (uc *CreateUseCase) Execute(ctx context.Context, in CreateInput) (TariffView, error) {
	var view TariffView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		subject, err := pricing.New(pricing.ID(uc.ids.NewID()), tenant.ID(in.TenantID), pricing.Draft{
			ProfessionalUserID: in.ProfessionalUserID,
			Coverage:           patient.Coverage(in.Coverage),
			InsurerID:          optionalString(in.InsurerID),
			Modality:           schedule.Modality(in.Modality),
			Currency:           patient.Currency(in.Currency),
			ValidFrom:          in.ValidFrom,
			InsurerMinor:       in.InsurerMinor,
			PatientMinor:       in.PatientMinor,
		}, uc.clock.Now())
		if err != nil {
			return err
		}

		insurerName, err := uc.activeInsurerName(ctx, subject)
		if err != nil {
			return err
		}
		if err := uc.repo.Create(ctx, subject); err != nil {
			return err
		}
		view = toView(subject, "", insurerName)
		return nil
	})
	if err != nil {
		return TariffView{}, err
	}
	return view, nil
}

// activeInsurerName resolves the mutual of a mutual price and refuses one the practice
// does not have or has stopped taking. A particular price has nothing to check.
func (uc *CreateUseCase) activeInsurerName(ctx context.Context, subject pricing.Tariff) (string, error) {
	if subject.InsurerID == nil {
		return "", nil
	}
	mutual, err := uc.insurers.ByID(ctx, insurer.ID(*subject.InsurerID))
	if err != nil {
		if errors.Is(err, insurer.ErrNotFound) {
			return "", &pricing.ValidationError{Field: "insurer_id", Message: "must be a mutual of this practice"}
		}
		return "", err
	}
	if !mutual.IsActive {
		return "", &pricing.ValidationError{Field: "insurer_id", Message: "the mutual is inactive: reactivate it to price it"}
	}
	return mutual.Name, nil
}

// UpdateInput changes the amounts of a price. Its key stays as it was created.
type UpdateInput struct {
	TenantID     string
	ActorID      string
	Role         string
	ID           string
	InsurerMinor int64
	PatientMinor int64
}

// UpdateUseCase edits the amounts of an existing price.
type UpdateUseCase struct {
	repo  port.TariffRepository
	scope port.TenantScope
	clock port.Clock
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo port.TariffRepository, scope port.TenantScope, clock port.Clock) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, scope: scope, clock: clock}
}

// Execute loads the price, applies the new split, and persists it.
func (uc *UpdateUseCase) Execute(ctx context.Context, in UpdateInput) (TariffView, error) {
	var view TariffView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		current, err := uc.repo.ByID(ctx, pricing.ID(in.ID))
		if err != nil {
			return err
		}
		updated, err := current.ChangeAmounts(pricing.Amounts{
			InsurerMinor: in.InsurerMinor,
			PatientMinor: in.PatientMinor,
		}, uc.clock.Now())
		if err != nil {
			return err
		}
		if err := uc.repo.Update(ctx, updated); err != nil {
			return err
		}
		// The names are not re-read: the screen reloads the schedule after a write.
		view = toView(updated, "", "")
		return nil
	})
	if err != nil {
		return TariffView{}, err
	}
	return view, nil
}

// DeleteInput identifies the price to remove.
type DeleteInput struct {
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// DeleteUseCase removes a price. A removed price is simply no longer proposed: the
// vouchers already issued keep the value they were issued with (T54).
type DeleteUseCase struct {
	repo  port.TariffRepository
	scope port.TenantScope
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo port.TariffRepository, scope port.TenantScope) *DeleteUseCase {
	return &DeleteUseCase{repo: repo, scope: scope}
}

// Execute removes the price inside the tenant scope.
func (uc *DeleteUseCase) Execute(ctx context.Context, in DeleteInput) error {
	return uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		return uc.repo.Delete(ctx, pricing.ID(in.ID))
	})
}

// optionalString turns an empty form value into absence, not a blank the domain would
// have to reject.
func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

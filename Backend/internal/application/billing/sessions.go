package billingapp

import (
	"context"
	"errors"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// SessionPageSize bounds the sessions a patient's emission screen lists: the newest ones.
const (
	DefaultSessionPageSize int32 = 50
	MaxSessionPageSize     int32 = 100
)

// PatientSessionsInput asks for the newest sessions of one patient.
type PatientSessionsInput struct {
	TenantID  string
	PatientID string
	Limit     int32
}

// PatientSessionsUseCase lists the sessions of a patient with the amount the tariff proposes
// for each one. The proposal is read here, not in the browser, so the screen and the server
// agree on which tariff prices a session.
type PatientSessionsUseCase struct {
	billing port.BillingRepository
	tariffs port.TariffRepository
	tenants port.TenantRepository
	scope   port.TenantScope
}

// NewPatientSessionsUseCase wires the use case.
func NewPatientSessionsUseCase(
	billing port.BillingRepository,
	tariffs port.TariffRepository,
	tenants port.TenantRepository,
	scope port.TenantScope,
) *PatientSessionsUseCase {
	return &PatientSessionsUseCase{billing: billing, tariffs: tariffs, tenants: tenants, scope: scope}
}

// Execute returns the sessions of the patient, newest first. A patient the practice does not
// hold is billing.ErrPatientNotVisible.
func (uc *PatientSessionsUseCase) Execute(ctx context.Context, in PatientSessionsInput) ([]AppointmentView, error) {
	limit := in.Limit
	if limit == 0 {
		limit = DefaultSessionPageSize
	}
	if limit < 1 || limit > MaxSessionPageSize {
		return nil, &billing.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}

	practice, err := uc.tenants.ByID(ctx, tenant.ID(in.TenantID))
	if err != nil {
		return nil, err
	}
	location, err := practiceLocation(practice.Timezone)
	if err != nil {
		return nil, err
	}

	var views []AppointmentView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		terms, err := uc.billing.PatientBilling(ctx, in.PatientID)
		if err != nil {
			return err
		}
		sessions, err := uc.billing.PatientAppointments(ctx, in.PatientID, limit)
		if err != nil {
			return err
		}
		views = make([]AppointmentView, 0, len(sessions))
		for _, session := range sessions {
			proposal, err := uc.proposal(ctx, terms, session, location)
			if err != nil {
				return err
			}
			views = append(views, toAppointmentView(session, proposal))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// proposal prices one session with the tariff in force on its civil day, in the practice's
// timezone. The amount is the total of the tariff: what the mutual pays plus the copago (for a
// particular session, the whole fee). A session with no tariff has no proposal.
func (uc *PatientSessionsUseCase) proposal(
	ctx context.Context,
	terms billing.PatientBilling,
	session billing.Appointment,
	location *time.Location,
) (*ProposalView, error) {
	at := session.StartsAt.In(location)
	tariff, err := uc.tariffs.InForce(ctx, port.TariffQuery{
		ProfessionalUserID: session.ProfessionalID,
		Coverage:           string(terms.Coverage),
		InsurerID:          terms.InsurerID,
		Modality:           session.Modality,
		Currency:           terms.Currency,
		Day:                time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC),
	})
	if errors.Is(err, pricing.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ProposalView{AmountMinor: tariff.InsurerMinor + tariff.PatientMinor, Currency: string(tariff.Currency)}, nil
}

// ProfessionalsUseCase lists the professionals a loose charge may name.
type ProfessionalsUseCase struct {
	billing port.BillingRepository
	scope   port.TenantScope
}

// NewProfessionalsUseCase wires the use case.
func NewProfessionalsUseCase(billing port.BillingRepository, scope port.TenantScope) *ProfessionalsUseCase {
	return &ProfessionalsUseCase{billing: billing, scope: scope}
}

// Execute returns the active professionals of the practice, by name.
func (uc *ProfessionalsUseCase) Execute(ctx context.Context, tenantID string) ([]ProfessionalView, error) {
	var views []ProfessionalView
	err := uc.scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		professionals, err := uc.billing.Professionals(ctx)
		if err != nil {
			return err
		}
		views = make([]ProfessionalView, 0, len(professionals))
		for _, professional := range professionals {
			views = append(views, toProfessionalView(professional))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// practiceLocation reads the practice's timezone. An unset one is the default timezone of the
// product, the same one a new practice is created with.
func practiceLocation(name string) (*time.Location, error) {
	if name == "" {
		name = tenant.DefaultTimezone
	}
	return time.LoadLocation(name)
}

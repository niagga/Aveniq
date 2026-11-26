package patientapp

import (
	"context"
	"errors"
	"strings"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	clientapp "github.com/niagga/aveniq/backend/internal/application/client"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// CreateInput is what the caller chooses for a new patient. Identity, timestamps
// and the active flag are absent because the system decides them.
type CreateInput struct {
	ProfileInput
	// ClientID names an existing billing client; when empty a new client is created
	// from ClientProfile (default payer is the patient themself).
	ClientID       string
	ClientProfile  clientapp.ProfileInput
	TenantID       string
	ActorID        string
	Role           string
}

// CreateUseCase registers a new patient.
type CreateUseCase struct {
	repo     port.PatientRepository
	clients  port.ClientRepository
	insurers port.InsurerRepository
	catalog  port.DocumentCatalog
	scope    port.TenantScope
	clock    port.Clock
	ids      port.IDGenerator
}

// NewCreateUseCase wires the use case through constructor injection: every
// dependency is explicit and replaceable, which is what keeps this layer testable
// without a database.
func NewCreateUseCase(repo port.PatientRepository, clients port.ClientRepository, insurers port.InsurerRepository, catalog port.DocumentCatalog, scope port.TenantScope, clock port.Clock, ids port.IDGenerator) *CreateUseCase {
	return &CreateUseCase{repo: repo, clients: clients, insurers: insurers, catalog: catalog, scope: scope, clock: clock, ids: ids}
}

// Execute creates the patient or returns a domain error. Normalization runs
// before the tenant transaction opens (see normalizeProfile); the write itself
// happens inside exactly one WithTenant, where row-level security reads
// app.tenant_id.
func (uc *CreateUseCase) Execute(ctx context.Context, in CreateInput) (PatientView, error) {
	if strings.TrimSpace(in.ClientID) != "" && clientInputGiven(in.ClientProfile) {
		return PatientView{}, &patient.ValidationError{Field: "client", Message: "must not be sent together with clientId"}
	}

	draft, err := normalizeProfile(ctx, uc.catalog, in.TenantID, in.ProfileInput)
	if err != nil {
		return PatientView{}, err
	}

	var view PatientView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		var payer client.Client
		if id := strings.TrimSpace(in.ClientID); id != "" {
			payer, err = uc.clients.ByID(ctx, client.ID(id))
			if errors.Is(err, client.ErrNotFound) {
				return &patient.ValidationError{Field: "client_id", Message: "must be a billing client of this practice"}
			}
			if err != nil {
				return err
			}
			if !payer.IsActive {
				return &patient.ValidationError{Field: "client_id", Message: "is deactivated: choose an active billing client"}
			}
		} else {
			profile := in.ClientProfile
			if strings.TrimSpace(profile.DisplayName) == "" {
				profile.DisplayName = strings.TrimSpace(in.FullName)
			}
			if strings.TrimSpace(profile.Country) == "" {
				profile.Country = draft.Country
			}
			clientDraft, err := clientapp.DraftFromProfile(ctx, uc.catalog, in.TenantID, profile)
			if err != nil {
				return err
			}
			payerID := client.ID(uc.ids.NewID())
			payer, err = client.New(payerID, tenant.ID(in.TenantID), clientDraft, uc.clock.Now())
			if err != nil {
				return err
			}
			if err := uc.clients.Create(ctx, payer); err != nil {
				return err
			}
		}

		subject, err := patient.New(patient.ID(uc.ids.NewID()), tenant.ID(in.TenantID), draft, uc.clock.Now())
		if err != nil {
			return err
		}
		if subject.CoverageKind == patient.CoverageInsurer {
			if err := requireActiveInsurer(ctx, uc.insurers, *subject.InsurerID); err != nil {
				return err
			}
		}
		if care.BecomesTreatingOnCreate(careapp.Viewer(in.ActorID, in.Role)) {
			subject = subject.WithTreating(in.ActorID)
		}
		subject.ClientID = string(payer.ID)

		if err := uc.repo.Create(ctx, subject); err != nil {
			return err
		}
		view = toView(subject, &payer)
		return nil
	})
	if err != nil {
		return PatientView{}, err
	}
	return view, nil
}

func clientInputGiven(in clientapp.ProfileInput) bool {
	return in.DisplayName != "" || in.LegalName != "" || in.Country != "" ||
		in.TaxIDKind != "" || in.TaxIDNumber != "" || in.FiscalCondition != "" ||
		in.FiscalAddress != "" || in.BillingCurrency != ""
}
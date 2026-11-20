package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	pricingapp "github.com/niagga/aveniq/backend/internal/application/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// tariffBody is one price of the fee schedule as the client sees it.
type tariffBody struct {
	ID                 string `json:"id" format:"uuid" doc:"Tariff identifier."`
	TenantID           string `json:"tenantId" format:"uuid" doc:"Practice the tariff belongs to."`
	ProfessionalUserID string `json:"professionalUserId" format:"uuid" doc:"Professional the price is for."`
	ProfessionalName   string `json:"professionalName" doc:"Name of the professional, as the practice has it."`
	Coverage           string `json:"coverage" enum:"private,insurer" doc:"private for a particular patient, insurer for a mutual."`
	InsurerID          string `json:"insurerId,omitempty" format:"uuid" doc:"Mutual of a mutual price. Absent for a particular one."`
	InsurerName        string `json:"insurerName,omitempty" doc:"Name of the mutual. Absent for a particular price."`
	Modality           string `json:"modality" enum:"online,presencial" doc:"Where the session happens."`
	BillingCurrency    string `json:"billingCurrency" enum:"ARS,EUR,USD" doc:"Currency the amounts are in."`
	ValidFrom          string `json:"validFrom" format:"date" doc:"First day the price applies. A later price for the same combination supersedes it from its own day."`
	InsurerMinor       int64  `json:"insurerMinor" doc:"What the mutual pays per session, in minor units. Zero for a particular price."`
	PatientMinor       int64  `json:"patientMinor" doc:"What the patient pays per session, in minor units. The whole fee for a particular price."`
	TotalMinor         int64  `json:"totalMinor" doc:"What one session is worth: insurerMinor plus patientMinor."`
	CreatedAt          string `json:"createdAt" format:"date-time"`
	UpdatedAt          string `json:"updatedAt" format:"date-time"`
}

// tariffCreateBody is a new price. Every amount is sent, zero included, so the client
// states the split instead of the server guessing it.
type tariffCreateBody struct {
	ProfessionalUserID string `json:"professionalUserId" format:"uuid"`
	Coverage           string `json:"coverage" enum:"private,insurer"`
	InsurerID          string `json:"insurerId,omitempty" format:"uuid" doc:"Required for a mutual price, absent for a particular one."`
	Modality           string `json:"modality" enum:"online,presencial"`
	BillingCurrency    string `json:"billingCurrency" enum:"ARS,EUR,USD"`
	ValidFrom          string `json:"validFrom" format:"date" doc:"YYYY-MM-DD."`
	InsurerMinor       int64  `json:"insurerMinor" minimum:"0" maximum:"1000000000000" doc:"Zero for a particular price."`
	PatientMinor       int64  `json:"patientMinor" minimum:"0" maximum:"1000000000000" doc:"The whole fee for a particular price."`
}

// tariffAmountsBody changes the amounts of a price. Its key is fixed at creation.
type tariffAmountsBody struct {
	InsurerMinor int64 `json:"insurerMinor" minimum:"0" maximum:"1000000000000"`
	PatientMinor int64 `json:"patientMinor" minimum:"0" maximum:"1000000000000"`
}

func toTariffBody(view pricingapp.TariffView) tariffBody {
	return tariffBody{
		ID:                 view.ID,
		TenantID:           view.TenantID,
		ProfessionalUserID: view.ProfessionalUserID,
		ProfessionalName:   view.ProfessionalName,
		Coverage:           view.Coverage,
		InsurerID:          view.InsurerID,
		InsurerName:        view.InsurerName,
		Modality:           view.Modality,
		BillingCurrency:    view.Currency,
		ValidFrom:          view.ValidFrom,
		InsurerMinor:       view.InsurerMinor,
		PatientMinor:       view.PatientMinor,
		TotalMinor:         view.TotalMinor,
		CreatedAt:          view.CreatedAt,
		UpdatedAt:          view.UpdatedAt,
	}
}

// ─── routes ──────────────────────────────────────────────────────────────────

type tariffHandlers struct {
	list   *pricingapp.ListUseCase
	create *pricingapp.CreateUseCase
	update *pricingapp.UpdateUseCase
	delete *pricingapp.DeleteUseCase
	logger *slog.Logger
}

// registerTariffRoutes declares the fee schedule endpoints.
//
// Reading takes billing:read: the accountant reads the schedule, and the vouchers start
// from it. Changing it takes tenant:settings, which only owner and admin hold, the same
// rule as the mutual catalog: the schedule is practice configuration.
func registerTariffRoutes(api huma.API, deps Deps) {
	handlers := &tariffHandlers{
		list:   deps.ListTariffs,
		create: deps.CreateTariff,
		update: deps.UpdateTariff,
		delete: deps.DeleteTariff,
		logger: deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermBillingRead)
	settingsGuard := permissionGuard(api, deps.AccessTokens, iam.PermTenantSettings)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-tariffs",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/tariffs",
		Summary:       "List the fee schedule",
		Description:   "Every price of the practice, ordered for reading by professional. A combination without a price has no row: nothing is proposed for it.",
		Tags:          []string{"tariffs"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listTariffs)

	huma.Register(api, huma.Operation{
		OperationID:   "create-tariff",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/tariffs",
		Summary:       "Add a price to the fee schedule",
		Description:   "Starts a price on validFrom for one professional, coverage, modality and currency. A price already in force keeps the sessions before that day.",
		Tags:          []string{"tariffs"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.createTariff)

	huma.Register(api, huma.Operation{
		OperationID:   "update-tariff",
		Method:        http.MethodPatch,
		Path:          APIPrefix + "/tariffs/{id}",
		Summary:       "Change the amounts of a price",
		Description:   "Replaces the split of a price. Its professional, coverage, modality, currency and start day do not change: to move one, remove it and create it again.",
		Tags:          []string{"tariffs"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.updateTariff)

	huma.Register(api, huma.Operation{
		OperationID:   "delete-tariff",
		Method:        http.MethodDelete,
		Path:          APIPrefix + "/tariffs/{id}",
		Summary:       "Remove a price",
		Description:   "Removes one price. Vouchers already issued keep the value they were issued with.",
		Tags:          []string{"tariffs"},
		DefaultStatus: http.StatusNoContent,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.deleteTariff)
}

func (h *tariffHandlers) listTariffs(ctx context.Context, _ *struct{}) (*tariffListOutput, error) {
	who := identityOf(ctx)
	views, err := h.list.Execute(ctx, pricingapp.ListInput{TenantID: who.TenantID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &tariffListOutput{}
	out.Body.Items = make([]tariffBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toTariffBody(view))
	}
	return out, nil
}

func (h *tariffHandlers) createTariff(ctx context.Context, in *tariffCreateInput) (*tariffOutput, error) {
	validFrom, err := time.Parse(dayLayout, in.Body.ValidFrom)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("validFrom must be a date, YYYY-MM-DD")
	}

	who := identityOf(ctx)
	view, err := h.create.Execute(ctx, pricingapp.CreateInput{
		TenantID:           who.TenantID,
		ActorID:            who.UserID,
		Role:               who.Role,
		ProfessionalUserID: in.Body.ProfessionalUserID,
		Coverage:           in.Body.Coverage,
		InsurerID:          in.Body.InsurerID,
		Modality:           in.Body.Modality,
		Currency:           in.Body.BillingCurrency,
		ValidFrom:          validFrom,
		InsurerMinor:       in.Body.InsurerMinor,
		PatientMinor:       in.Body.PatientMinor,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &tariffOutput{Body: toTariffBody(view)}, nil
}

func (h *tariffHandlers) updateTariff(ctx context.Context, in *tariffUpdateInput) (*tariffOutput, error) {
	who := identityOf(ctx)
	view, err := h.update.Execute(ctx, pricingapp.UpdateInput{
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		Role:         who.Role,
		ID:           in.ID,
		InsurerMinor: in.Body.InsurerMinor,
		PatientMinor: in.Body.PatientMinor,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &tariffOutput{Body: toTariffBody(view)}, nil
}

func (h *tariffHandlers) deleteTariff(ctx context.Context, in *tariffIDInput) (*struct{}, error) {
	who := identityOf(ctx)
	if err := h.delete.Execute(ctx, pricingapp.DeleteInput{
		TenantID: who.TenantID,
		ActorID:  who.UserID,
		Role:     who.Role,
		ID:       in.ID,
	}); err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return nil, nil
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

const dayLayout = "2006-01-02"

type tariffIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Tariff identifier."`
}

type tariffCreateInput struct {
	Body tariffCreateBody
}

type tariffUpdateInput struct {
	ID   string `path:"id" format:"uuid" doc:"Tariff identifier."`
	Body tariffAmountsBody
}

type tariffOutput struct {
	Body tariffBody
}

type tariffListOutput struct {
	Body struct {
		Items []tariffBody `json:"items"`
	}
}

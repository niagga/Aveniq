package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	clientapp "github.com/niagga/aveniq/backend/internal/application/client"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

type clientBody struct {
	ID              string `json:"id" format:"uuid"`
	TenantID        string `json:"tenantId" format:"uuid"`
	DisplayName     string `json:"displayName"`
	LegalName       string `json:"legalName,omitempty"`
	Country         string `json:"country"`
	TaxIDKind       string `json:"taxIdKind,omitempty"`
	TaxIDNumber     string `json:"taxIdNumber,omitempty"`
	FiscalCondition string `json:"fiscalCondition,omitempty"`
	FiscalAddress   string `json:"fiscalAddress,omitempty"`
	BillingCurrency string `json:"billingCurrency"`
	IsActive        bool   `json:"isActive"`
	CreatedAt       string `json:"createdAt" format:"date-time"`
	UpdatedAt       string `json:"updatedAt" format:"date-time"`
}

type clientProfileBody struct {
	DisplayName     string `json:"displayName" minLength:"1" maxLength:"160"`
	LegalName       string `json:"legalName,omitempty" maxLength:"160"`
	Country         string `json:"country,omitempty" minLength:"2" maxLength:"2"`
	TaxIDKind       string `json:"taxIdKind,omitempty"`
	TaxIDNumber     string `json:"taxIdNumber,omitempty"`
	FiscalCondition string `json:"fiscalCondition,omitempty"`
	FiscalAddress   string `json:"fiscalAddress,omitempty" maxLength:"240"`
	BillingCurrency string `json:"billingCurrency"`
}

func (b clientProfileBody) profile() clientapp.ProfileInput {
	return clientapp.ProfileInput{
		DisplayName:     b.DisplayName,
		LegalName:       b.LegalName,
		Country:         b.Country,
		TaxIDKind:       b.TaxIDKind,
		TaxIDNumber:     b.TaxIDNumber,
		FiscalCondition: b.FiscalCondition,
		FiscalAddress:   b.FiscalAddress,
		BillingCurrency: b.BillingCurrency,
	}
}

func toClientBody(view clientapp.ClientView) clientBody {
	return clientBody{
		ID:              view.ID,
		TenantID:        view.TenantID,
		DisplayName:     view.DisplayName,
		LegalName:       view.LegalName,
		Country:         view.Country,
		TaxIDKind:       view.TaxIDKind,
		TaxIDNumber:     view.TaxIDNumber,
		FiscalCondition: view.FiscalCondition,
		FiscalAddress:   view.FiscalAddress,
		BillingCurrency: view.BillingCurrency,
		IsActive:        view.IsActive,
		CreatedAt:       view.CreatedAt,
		UpdatedAt:       view.UpdatedAt,
	}
}

type clientHandlers struct {
	list       *clientapp.ListUseCase
	get        *clientapp.GetUseCase
	create     *clientapp.CreateUseCase
	update     *clientapp.UpdateUseCase
	deactivate *clientapp.LifecycleUseCase
	activate   *clientapp.LifecycleUseCase
	logger     *slog.Logger
}

func registerClientRoutes(api huma.API, deps Deps) {
	handlers := &clientHandlers{
		list:       deps.ListClients,
		get:        deps.GetClient,
		create:     deps.CreateClient,
		update:     deps.UpdateClient,
		deactivate: deps.DeactivateClient,
		activate:   deps.ActivateClient,
		logger:     deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermClientRead)
	writeGuard := permissionGuard(api, deps.AccessTokens, iam.PermClientWrite)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-clients",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/clients",
		Summary:       "List billing clients",
		Description:   "Returns who pays for the patients of the practice. Order by display name.",
		Tags:          []string{"clients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listClients)

	huma.Register(api, huma.Operation{
		OperationID:   "get-client",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/clients/{id}",
		Summary:       "Read a billing client",
		Tags:          []string{"clients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.getClient)

	huma.Register(api, huma.Operation{
		OperationID:   "create-client",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/clients",
		Summary:       "Register a billing client",
		Tags:          []string{"clients"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.createClient)

	huma.Register(api, huma.Operation{
		OperationID:   "update-client",
		Method:        http.MethodPatch,
		Path:          APIPrefix + "/clients/{id}",
		Summary:       "Edit a billing client",
		Tags:          []string{"clients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.updateClient)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivate-client",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/clients/{id}/deactivate",
		Summary:       "Deactivate a billing client",
		Tags:          []string{"clients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.deactivateClient)

	huma.Register(api, huma.Operation{
		OperationID:   "activate-client",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/clients/{id}/activate",
		Summary:       "Reactivate a billing client",
		Tags:          []string{"clients"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   writeGuard,
	}, handlers.activateClient)
}

func (h *clientHandlers) listClients(ctx context.Context, in *listClientsInput) (*listClientsOutput, error) {
	who := identityOf(ctx)
	views, err := h.list.Execute(ctx, clientapp.ListInput{TenantID: who.TenantID, ActiveOnly: in.ActiveOnly})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	out := &listClientsOutput{}
	out.Body.Items = make([]clientBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toClientBody(view))
	}
	return out, nil
}

func (h *clientHandlers) getClient(ctx context.Context, in *clientIDInput) (*clientOutput, error) {
	who := identityOf(ctx)
	view, err := h.get.Execute(ctx, clientapp.GetInput{TenantID: who.TenantID, ID: in.ID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &clientOutput{Body: toClientBody(view)}, nil
}

func (h *clientHandlers) createClient(ctx context.Context, in *createClientInput) (*clientOutput, error) {
	who := identityOf(ctx)
	view, err := h.create.Execute(ctx, clientapp.CreateInput{
		ProfileInput: in.Body.profile(),
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		Role:         who.Role,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &clientOutput{Body: toClientBody(view)}, nil
}

func (h *clientHandlers) updateClient(ctx context.Context, in *updateClientInput) (*clientOutput, error) {
	who := identityOf(ctx)
	view, err := h.update.Execute(ctx, clientapp.UpdateInput{
		ProfileInput: in.Body.profile(),
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		Role:         who.Role,
		ID:           in.ID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &clientOutput{Body: toClientBody(view)}, nil
}

func (h *clientHandlers) deactivateClient(ctx context.Context, in *clientIDInput) (*clientOutput, error) {
	return h.lifecycle(ctx, h.deactivate, in.ID)
}

func (h *clientHandlers) activateClient(ctx context.Context, in *clientIDInput) (*clientOutput, error) {
	return h.lifecycle(ctx, h.activate, in.ID)
}

func (h *clientHandlers) lifecycle(ctx context.Context, useCase *clientapp.LifecycleUseCase, id string) (*clientOutput, error) {
	who := identityOf(ctx)
	view, err := useCase.Execute(ctx, clientapp.LifecycleInput{
		TenantID: who.TenantID,
		ActorID:  who.UserID,
		Role:     who.Role,
		ID:       id,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &clientOutput{Body: toClientBody(view)}, nil
}

type listClientsInput struct {
	ActiveOnly bool `query:"activeOnly" default:"false"`
}

type createClientInput struct {
	Body clientProfileBody
}

type updateClientInput struct {
	ID   string `path:"id" format:"uuid"`
	Body clientProfileBody
}

type clientIDInput struct {
	ID string `path:"id" format:"uuid"`
}

type clientOutput struct {
	Body clientBody
}

type listClientsOutput struct {
	Body struct {
		Items []clientBody `json:"items"`
	}
}
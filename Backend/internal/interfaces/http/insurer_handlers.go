package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	insurerapp "github.com/niagga/aveniq/backend/internal/application/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// insurerBody is a mutual of the practice's catalog as the client sees it.
type insurerBody struct {
	ID        string `json:"id" format:"uuid" doc:"Mutual identifier."`
	TenantID  string `json:"tenantId" format:"uuid" doc:"Practice the mutual belongs to."`
	Name      string `json:"name" doc:"Name of the mutual as the practice calls it."`
	TaxID     string `json:"taxId,omitempty" doc:"CUIT of the mutual, digits only. Empty until the practice knows it."`
	IsActive  bool   `json:"isActive" doc:"False after a deactivation. Mutuals are never deleted."`
	CreatedAt string `json:"createdAt" format:"date-time"`
	UpdatedAt string `json:"updatedAt" format:"date-time"`
}

// insurerProfileBody is what a client may send to create or edit a mutual.
type insurerProfileBody struct {
	Name  string `json:"name" minLength:"1" maxLength:"120" doc:"Name of the mutual."`
	TaxID string `json:"taxId,omitempty" maxLength:"20" doc:"CUIT of the mutual. Separators are accepted and removed."`
}

func (b insurerProfileBody) profile() insurerapp.ProfileInput {
	return insurerapp.ProfileInput{Name: b.Name, TaxID: b.TaxID}
}

func toInsurerBody(view insurerapp.InsurerView) insurerBody {
	return insurerBody{
		ID:        view.ID,
		TenantID:  view.TenantID,
		Name:      view.Name,
		TaxID:     view.TaxID,
		IsActive:  view.IsActive,
		CreatedAt: view.CreatedAt,
		UpdatedAt: view.UpdatedAt,
	}
}

// ─── routes ──────────────────────────────────────────────────────────────────

type insurerHandlers struct {
	list       *insurerapp.ListUseCase
	create     *insurerapp.CreateUseCase
	update     *insurerapp.UpdateUseCase
	deactivate *insurerapp.LifecycleUseCase
	activate   *insurerapp.LifecycleUseCase
	logger     *slog.Logger
}

// registerInsurerRoutes declares the mutual catalog endpoints.
//
// Reading the catalog takes patients:read, because registering a patient needs to choose
// a mutual. Changing it takes tenant:settings, which only owner and admin hold: the
// catalog is practice configuration, not a clinical or reception task. Mutuals are
// deactivated, never deleted.
func registerInsurerRoutes(api huma.API, deps Deps) {
	handlers := &insurerHandlers{
		list:       deps.ListInsurers,
		create:     deps.CreateInsurer,
		update:     deps.UpdateInsurer,
		deactivate: deps.DeactivateInsurer,
		activate:   deps.ActivateInsurer,
		logger:     deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermPatientsRead)
	settingsGuard := permissionGuard(api, deps.AccessTokens, iam.PermTenantSettings)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-insurers",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/insurers",
		Summary:       "List the mutuals of the practice",
		Description:   "Returns the practice's mutuals ordered by name. The patient forms ask for the active ones only.",
		Tags:          []string{"insurers"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listInsurers)

	huma.Register(api, huma.Operation{
		OperationID:   "create-insurer",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/insurers",
		Summary:       "Add a mutual to the catalog",
		Tags:          []string{"insurers"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.createInsurer)

	huma.Register(api, huma.Operation{
		OperationID:   "update-insurer",
		Method:        http.MethodPatch,
		Path:          APIPrefix + "/insurers/{id}",
		Summary:       "Edit a mutual",
		Description:   "Replaces the name and the tax id of a mutual. The active flag is changed by the deactivate and activate routes.",
		Tags:          []string{"insurers"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.updateInsurer)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivate-insurer",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/insurers/{id}/deactivate",
		Summary:       "Deactivate a mutual",
		Description:   "Stops new coverage on the mutual. Patients it already covers keep it until someone changes their coverage.",
		Tags:          []string{"insurers"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.deactivateInsurer)

	huma.Register(api, huma.Operation{
		OperationID:   "activate-insurer",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/insurers/{id}/activate",
		Summary:       "Reactivate a mutual",
		Tags:          []string{"insurers"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.activateInsurer)
}

func (h *insurerHandlers) listInsurers(ctx context.Context, in *listInsurersInput) (*listInsurersOutput, error) {
	who := identityOf(ctx)
	views, err := h.list.Execute(ctx, insurerapp.ListInput{TenantID: who.TenantID, ActiveOnly: in.ActiveOnly})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listInsurersOutput{}
	out.Body.Items = make([]insurerBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toInsurerBody(view))
	}
	return out, nil
}

func (h *insurerHandlers) createInsurer(ctx context.Context, in *createInsurerInput) (*insurerOutput, error) {
	who := identityOf(ctx)
	view, err := h.create.Execute(ctx, insurerapp.CreateInput{
		ProfileInput: in.Body.profile(),
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		Role:         who.Role,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &insurerOutput{Body: toInsurerBody(view)}, nil
}

func (h *insurerHandlers) updateInsurer(ctx context.Context, in *updateInsurerInput) (*insurerOutput, error) {
	who := identityOf(ctx)
	view, err := h.update.Execute(ctx, insurerapp.UpdateInput{
		ProfileInput: in.Body.profile(),
		TenantID:     who.TenantID,
		ActorID:      who.UserID,
		Role:         who.Role,
		ID:           in.ID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &insurerOutput{Body: toInsurerBody(view)}, nil
}

func (h *insurerHandlers) deactivateInsurer(ctx context.Context, in *insurerIDInput) (*insurerOutput, error) {
	return h.lifecycle(ctx, h.deactivate, in.ID)
}

func (h *insurerHandlers) activateInsurer(ctx context.Context, in *insurerIDInput) (*insurerOutput, error) {
	return h.lifecycle(ctx, h.activate, in.ID)
}

func (h *insurerHandlers) lifecycle(ctx context.Context, useCase *insurerapp.LifecycleUseCase, id string) (*insurerOutput, error) {
	who := identityOf(ctx)
	view, err := useCase.Execute(ctx, insurerapp.LifecycleInput{
		TenantID: who.TenantID,
		ActorID:  who.UserID,
		Role:     who.Role,
		ID:       id,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &insurerOutput{Body: toInsurerBody(view)}, nil
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type listInsurersInput struct {
	ActiveOnly bool `query:"activeOnly" default:"false" doc:"Exclude deactivated mutuals."`
}

type createInsurerInput struct {
	Body insurerProfileBody
}

type updateInsurerInput struct {
	ID   string `path:"id" format:"uuid" doc:"Mutual identifier."`
	Body insurerProfileBody
}

type insurerIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Mutual identifier."`
}

type insurerOutput struct {
	Body insurerBody
}

type listInsurersOutput struct {
	Body struct {
		Items []insurerBody `json:"items"`
	}
}

package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
)

// tenantBody is the wire representation of a tenant. It is a deliberate copy of
// the application's read model rather than a reuse of it: the transport contract
// must be free to diverge from the internal shape.
type tenantBody struct {
	ID        string `json:"id" format:"uuid" doc:"Tenant identifier (UUIDv7)."`
	Slug      string `json:"slug" doc:"DNS label that resolves this tenant as a subdomain."`
	Name      string `json:"name" doc:"Display name of the practice."`
	Status    string `json:"status" enum:"trial,active,suspended" doc:"Lifecycle state."`
	CreatedAt string `json:"createdAt" format:"date-time" doc:"Creation timestamp, RFC 3339 in UTC."`
}

func toTenantBody(view tenantapp.TenantView) tenantBody {
	return tenantBody{
		ID:        view.ID,
		Slug:      view.Slug,
		Name:      view.Name,
		Status:    view.Status,
		CreatedAt: view.CreatedAt,
	}
}

type tenantHandlers struct {
	create *tenantapp.CreateUseCase
	list   *tenantapp.ListUseCase
	get    *tenantapp.GetBySlugUseCase
	logger *slog.Logger
}

// registerTenantRoutes declares the tenant endpoints. Each huma.Operation becomes
// both an HTTP route and an entry in the OpenAPI document.
func registerTenantRoutes(api huma.API, deps Deps) {
	handlers := &tenantHandlers{
		create: deps.CreateTenant,
		list:   deps.ListTenants,
		get:    deps.GetTenant,
		logger: deps.Logger,
	}

	huma.Register(api, huma.Operation{
		OperationID:   "list-tenants",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/tenants",
		Summary:       "List tenants",
		Description:   "Returns one page of tenants, newest first.",
		Tags:          []string{"tenants"},
		DefaultStatus: http.StatusOK,
	}, handlers.listTenants)

	huma.Register(api, huma.Operation{
		OperationID:   "create-tenant",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/tenants",
		Summary:       "Register a tenant",
		Description:   "Creates a tenant in trial state. Slugs are unique platform-wide.",
		Tags:          []string{"tenants"},
		DefaultStatus: http.StatusCreated,
	}, handlers.createTenant)

	huma.Register(api, huma.Operation{
		OperationID:   "get-tenant",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/tenants/{slug}",
		Summary:       "Get a tenant by slug",
		Description:   "Resolves the tenant that a subdomain points at.",
		Tags:          []string{"tenants"},
		DefaultStatus: http.StatusOK,
	}, handlers.getTenant)
}

// ─── list ────────────────────────────────────────────────────────────────────

type listTenantsInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size."`
	Offset int32 `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
}

type listTenantsOutput struct {
	Body struct {
		Items []tenantBody `json:"items"`
	}
}

func (h *tenantHandlers) listTenants(ctx context.Context, in *listTenantsInput) (*listTenantsOutput, error) {
	result, err := h.list.Execute(ctx, tenantapp.ListInput{Limit: in.Limit, Offset: in.Offset})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	items := make([]tenantBody, 0, len(result.Tenants))
	for _, view := range result.Tenants {
		items = append(items, toTenantBody(view))
	}

	out := &listTenantsOutput{}
	out.Body.Items = items
	return out, nil
}

// ─── create ──────────────────────────────────────────────────────────────────

type createTenantInput struct {
	Body struct {
		Slug string `json:"slug" minLength:"3" maxLength:"63" pattern:"^[a-z0-9]+(-[a-z0-9]+)*$" example:"consultorio-sur" doc:"Lowercase DNS label: letters, digits and single inner hyphens."`
		Name string `json:"name" minLength:"1" maxLength:"120" example:"Consultorio Sur" doc:"Display name of the practice."`
	}
}

type createTenantOutput struct {
	Location string     `header:"Location" doc:"URL of the created tenant."`
	Body     tenantBody
}

func (h *tenantHandlers) createTenant(ctx context.Context, in *createTenantInput) (*createTenantOutput, error) {
	view, err := h.create.Execute(ctx, tenantapp.CreateInput{
		Slug: in.Body.Slug,
		Name: in.Body.Name,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	return &createTenantOutput{
		Location: APIPrefix + "/tenants/" + view.Slug,
		Body:     toTenantBody(view),
	}, nil
}

// ─── get ─────────────────────────────────────────────────────────────────────

type getTenantInput struct {
	Slug string `path:"slug" pattern:"^[a-zA-Z0-9-]+$" doc:"Tenant slug."`
}

type getTenantOutput struct {
	Body tenantBody
}

func (h *tenantHandlers) getTenant(ctx context.Context, in *getTenantInput) (*getTenantOutput, error) {
	view, err := h.get.Execute(ctx, tenantapp.GetBySlugInput{Slug: in.Slug})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	return &getTenantOutput{Body: toTenantBody(view)}, nil
}

// jsonEncoder returns a configured encoder; separated so every JSON response is
// written with the same settings.
func jsonEncoder(w io.Writer) *json.Encoder {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	return encoder
}

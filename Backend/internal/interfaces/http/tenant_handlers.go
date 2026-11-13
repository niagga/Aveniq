package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// tenantBody is the wire representation of a tenant. It is a deliberate copy of
// the application's read model rather than a reuse of it: the transport contract
// must be free to diverge from the internal shape.
type tenantBody struct {
	ID          string `json:"id" format:"uuid" doc:"Tenant identifier (UUIDv7)."`
	Slug        string `json:"slug" doc:"DNS label that resolves this tenant as a subdomain."`
	Name        string `json:"name" doc:"Display name of the practice."`
	Status      string `json:"status" enum:"trial,active,suspended" doc:"Lifecycle state."`
	Timezone    string `json:"timezone" doc:"IANA timezone used to expand recurring appointments."`
	CreatedAt   string `json:"createdAt" format:"date-time" doc:"Creation timestamp, RFC 3339 in UTC."`
	TrialEndsAt string `json:"trialEndsAt" format:"date-time" doc:"End of the trial, RFC 3339 in UTC. Set at registration, 14 days after creation."`
}

func toTenantBody(view tenantapp.TenantView) tenantBody {
	return tenantBody{
		ID:          view.ID,
		Slug:        view.Slug,
		Name:        view.Name,
		Status:      view.Status,
		Timezone:    view.Timezone,
		CreatedAt:   view.CreatedAt,
		TrialEndsAt: view.TrialEndsAt,
	}
}

type tenantHandlers struct {
	getByID *tenantapp.GetByIDUseCase
	logger  *slog.Logger
}

// registerTenantRoutes declares the tenant endpoints.
//
// Both reads are authenticated and identity-scoped: the practice comes from the
// signed token, never from the URL or the body, so a session cannot ask for another
// practice's row. There is no creation route here: a practice comes into existence
// only through the public registration (signup), which also creates its owner.
func registerTenantRoutes(api huma.API, deps Deps) {
	handlers := &tenantHandlers{
		getByID: deps.GetTenantByID,
		logger:  deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermTenantRead)

	huma.Register(api, huma.Operation{
		OperationID:   "list-tenants",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/tenants",
		Summary:       "Describe the current practice",
		Description:   "Returns the practice the session belongs to. It is a list for compatibility with the client, but a session can only ever see one: the platform-wide view belongs to an operator surface that does not exist yet.",
		Tags:          []string{"tenants"},
		DefaultStatus: http.StatusOK,
		Security:      []map[string][]string{{securitySchemeBearer: {}}},
		Middlewares:   readGuard,
	}, handlers.listTenants)

	huma.Register(api, huma.Operation{
		OperationID:   "get-tenant",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/tenants/{slug}",
		Summary:       "Get the current practice by slug",
		Description:   "Resolves the practice the session belongs to. Any other slug answers 404, not 403: confirming that another practice exists is itself information.",
		Tags:          []string{"tenants"},
		DefaultStatus: http.StatusOK,
		Security:      []map[string][]string{{securitySchemeBearer: {}}},
		Middlewares:   readGuard,
	}, handlers.getTenant)
}

// ─── list ────────────────────────────────────────────────────────────────────

type listTenantsOutput struct {
	Body struct {
		Items []tenantBody `json:"items"`
	}
}

func (h *tenantHandlers) listTenants(ctx context.Context, _ *struct{}) (*listTenantsOutput, error) {
	view, err := h.currentTenant(ctx)
	if err != nil {
		return nil, err
	}

	out := &listTenantsOutput{}
	out.Body.Items = []tenantBody{toTenantBody(view)}
	return out, nil
}

// currentTenant resolves the practice named by the verified token.
//
// A missing identity here is a wiring defect, not a request problem: the route is
// registered behind the guard, so reaching this point without one means the route
// was declared without it.
func (h *tenantHandlers) currentTenant(ctx context.Context) (tenantapp.TenantView, error) {
	identity, ok := IdentityFrom(ctx)
	if !ok {
		h.logger.ErrorContext(ctx, "identity-scoped route reached without an identity")
		return tenantapp.TenantView{}, huma.Error500InternalServerError("Unexpected error")
	}

	view, err := h.getByID.Execute(ctx, tenantapp.GetByIDInput{ID: identity.TenantID})
	if err != nil {
		return tenantapp.TenantView{}, problem(ctx, h.logger, err)
	}
	return view, nil
}

// ─── get ─────────────────────────────────────────────────────────────────────

type getTenantInput struct {
	Slug string `path:"slug" pattern:"^[a-zA-Z0-9-]+$" doc:"Tenant slug."`
}

type getTenantOutput struct {
	Body tenantBody
}

func (h *tenantHandlers) getTenant(ctx context.Context, in *getTenantInput) (*getTenantOutput, error) {
	view, err := h.currentTenant(ctx)
	if err != nil {
		return nil, err
	}

	// A slug belonging to another practice answers exactly what a nonexistent one
	// answers. Anything else would turn this route into an existence oracle.
	if view.Slug != in.Slug {
		return nil, huma.Error404NotFound("The requested tenant does not exist")
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

package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
)

// practiceStatusBody is the state of the caller's practice that every role may read.
type practiceStatusBody struct {
	Status      string `json:"status" enum:"trial,active,suspended" doc:"Lifecycle state."`
	Timezone    string `json:"timezone" doc:"IANA time zone in which the practice reads its days."`
	TrialEndsAt string `json:"trialEndsAt" format:"date-time" doc:"End of the trial, RFC 3339 in UTC."`
	Expired     bool   `json:"expired" doc:"True when the practice is on trial and the trial has ended: it reads, it does not write. Decided on the server's clock."`
}

type practiceStatusOutput struct {
	Body practiceStatusBody
}

type practiceStatusHandlers struct {
	status *tenantapp.PracticeStatusUseCase
	logger *slog.Logger
}

// registerPracticeStatusRoutes declares the read of the practice's state. It is open to any
// authenticated role on purpose: the accountant and the other roles without tenant:read still
// need to know that the trial has ended, so their screens stop offering writes the API will
// refuse (L59). The practice comes from the token, never from the request.
func registerPracticeStatusRoutes(api huma.API, deps Deps) {
	handlers := &practiceStatusHandlers{status: deps.PracticeStatus, logger: deps.Logger}

	huma.Register(api, huma.Operation{
		OperationID:   "get-practice-status",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/practice/status",
		Summary:       "Read the state of the caller's practice",
		Description:   "Status, time zone and trial of the practice the session belongs to, for every role. It names no one and holds nothing the roles without tenant:read should not see.",
		Tags:          []string{"tenants"},
		DefaultStatus: http.StatusOK,
		Security:      []map[string][]string{{securitySchemeBearer: {}}},
		Middlewares:   huma.Middlewares{authenticate(api, deps.AccessTokens)},
	}, handlers.get)
}

func (h *practiceStatusHandlers) get(ctx context.Context, _ *struct{}) (*practiceStatusOutput, error) {
	identity, ok := IdentityFrom(ctx)
	if !ok {
		h.logger.ErrorContext(ctx, "identity-scoped route reached without an identity")
		return nil, huma.Error500InternalServerError("Unexpected error")
	}

	view, err := h.status.Execute(ctx, identity.TenantID)
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &practiceStatusOutput{Body: practiceStatusBody{
		Status:      view.Status,
		Timezone:    view.Timezone,
		TrialEndsAt: view.TrialEndsAt,
		Expired:     view.Expired,
	}}, nil
}

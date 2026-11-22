package http

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	landingapp "github.com/niagga/aveniq/backend/internal/application/landing"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// landingBody is the landing of a practice as the client reads it. Every text field is always
// present, empty when the practice has not written it, so the screen never checks for absence.
type landingBody struct {
	Slug        string `json:"slug" doc:"Subdomain label of the practice: its landing lives at <slug>.<base domain>."`
	DisplayName string `json:"displayName" doc:"Name the landing shows as the practice's name."`
	Tagline     string `json:"tagline" doc:"One line under the name."`
	About       string `json:"about" doc:"What the practice does, in the owner's words."`
	Address     string `json:"address" doc:"Where the practice receives patients."`
	Phone       string `json:"phone" doc:"Phone number, digits and punctuation only."`
	Email       string `json:"email" doc:"Bare email address to write to."`
	Schedule    string `json:"schedule" doc:"Hours the practice receives patients, as the owner writes them."`
}

// landingDraftBody is what the owner sends to save the landing. A text field left out is saved
// as empty: the save replaces the whole landing, it does not patch it.
type landingDraftBody struct {
	DisplayName string `json:"displayName" doc:"Name the landing shows. Required."`
	Tagline     string `json:"tagline,omitempty" doc:"One line under the name."`
	About       string `json:"about,omitempty" doc:"What the practice does."`
	Address     string `json:"address,omitempty" doc:"Where the practice receives patients."`
	Phone       string `json:"phone,omitempty" doc:"Phone number, digits and punctuation only."`
	Email       string `json:"email,omitempty" doc:"Bare email address to write to."`
	Schedule    string `json:"schedule,omitempty" doc:"Hours the practice receives patients."`
}

func (b landingDraftBody) draft() landingapp.DraftInput {
	return landingapp.DraftInput{
		DisplayName: b.DisplayName,
		Tagline:     b.Tagline,
		About:       b.About,
		Address:     b.Address,
		Phone:       b.Phone,
		Email:       b.Email,
		Schedule:    b.Schedule,
	}
}

func toLandingBody(view landingapp.View) landingBody {
	return landingBody{
		Slug:        view.Slug,
		DisplayName: view.DisplayName,
		Tagline:     view.Tagline,
		About:       view.About,
		Address:     view.Address,
		Phone:       view.Phone,
		Email:       view.Email,
		Schedule:    view.Schedule,
	}
}

// ─── routes ──────────────────────────────────────────────────────────────────

type landingHandlers struct {
	public *landingapp.PublicUseCase
	get    *landingapp.GetUseCase
	update *landingapp.UpdateUseCase
	logger *slog.Logger
}

// registerLandingRoutes declares the landing endpoints.
//
// The public read takes no session: the public site asks it by host, and it is rate limited
// per client like the other public doors. Reading and editing the practice's own landing take
// tenant:settings, which only owner and admin hold: the landing is what the practice tells the
// world, the same kind of configuration as the mutual catalog.
func registerLandingRoutes(api huma.API, deps Deps) {
	handlers := &landingHandlers{
		public: deps.PublicLanding,
		get:    deps.GetLanding,
		update: deps.UpdateLanding,
		logger: deps.Logger,
	}

	settingsGuard := permissionGuard(api, deps.AccessTokens, iam.PermTenantSettings)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "get-public-landing",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/public/landing",
		Summary:       "Read the public landing of the practice at a host",
		Description:   "Answers the landing of the practice that owns the host: its slug subdomain under the base domain, or a custom domain. Only the published text is returned. A practice that is unknown, belongs to another domain, or is suspended answers the same 404. Rate limited per client.",
		Tags:          []string{"landing"},
		DefaultStatus: http.StatusOK,
		Middlewares:   rateLimited(api, deps.RateLimiter, deps.Logger, landingReadPolicy),
	}, handlers.publicLanding)

	huma.Register(api, huma.Operation{
		OperationID:   "get-practice-landing",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/practice/landing",
		Summary:       "Read the landing of the practice for editing",
		Description:   "Returns the landing of the caller's practice, or the default one (its name only) before the first save.",
		Tags:          []string{"landing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.practiceLanding)

	huma.Register(api, huma.Operation{
		OperationID:   "update-practice-landing",
		Method:        http.MethodPut,
		Path:          APIPrefix + "/practice/landing",
		Summary:       "Save the landing of the practice",
		Description:   "Replaces the landing text of the caller's practice. The save is recorded in the audit trail. Refused with 403 trial_expired once the trial has ended.",
		Tags:          []string{"landing"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   settingsGuard,
	}, handlers.updateLanding)
}

func (h *landingHandlers) publicLanding(ctx context.Context, in *publicLandingInput) (*landingOutput, error) {
	view, err := h.public.Execute(ctx, landingapp.PublicInput{Host: in.Host})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &landingOutput{Body: toLandingBody(view)}, nil
}

func (h *landingHandlers) practiceLanding(ctx context.Context, _ *struct{}) (*landingOutput, error) {
	who := identityOf(ctx)
	view, err := h.get.Execute(ctx, landingapp.GetInput{TenantID: who.TenantID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &landingOutput{Body: toLandingBody(view)}, nil
}

func (h *landingHandlers) updateLanding(ctx context.Context, in *updateLandingInput) (*landingOutput, error) {
	who := identityOf(ctx)
	view, err := h.update.Execute(ctx, landingapp.UpdateInput{
		DraftInput: in.Body.draft(),
		TenantID:   who.TenantID,
		ActorID:    who.UserID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &landingOutput{Body: toLandingBody(view)}, nil
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type publicLandingInput struct {
	Host string `query:"host" required:"true" doc:"The host the landing is requested for, as the browser sent it. A port is ignored."`
}

type updateLandingInput struct {
	Body landingDraftBody
}

type landingOutput struct {
	Body landingBody
}

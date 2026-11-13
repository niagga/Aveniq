package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	signupapp "github.com/niagga/aveniq/backend/internal/application/signup"
)

// signupOwnerBody is the first account of the new practice. EmailVerified is false
// on purpose: verification waits for a mail provider, and the client must not show
// the practice as verified before then.
type signupOwnerBody struct {
	ID            string `json:"id" format:"uuid" doc:"Account identifier of the owner."`
	Email         string `json:"email" doc:"Canonical login email of the owner."`
	Role          string `json:"role" enum:"owner" doc:"Always owner: the registrant is the first owner."`
	EmailVerified bool   `json:"emailVerified" doc:"False until the verification flow exists."`
}

// signupBody answers a registration with the practice and its owner. No session is
// issued: the owner signs in with the password they chose.
type signupBody struct {
	Tenant tenantBody      `json:"tenant"`
	Owner  signupOwnerBody `json:"owner"`
}

type signupHandlers struct {
	signup *signupapp.UseCase
	logger *slog.Logger
}

// registerSignupRoutes declares the public registration. It is rate limited per
// client because it is the one unauthenticated route that creates rows.
func registerSignupRoutes(api huma.API, deps Deps) {
	handlers := &signupHandlers{signup: deps.Signup, logger: deps.Logger}

	huma.Register(api, huma.Operation{
		OperationID:   "register-practice",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/signup",
		Summary:       "Register a practice in trial",
		Description:   "Public. Creates the practice in trial for 14 days and its first account, the owner, with the password chosen. Email verification is not available yet, so the owner is returned with emailVerified false. Answers 409 when the slug is taken, 422 for an invalid field or a password under 12 characters, and 429 after five attempts an hour from one client.",
		Tags:          []string{"signup"},
		DefaultStatus: http.StatusCreated,
		Middlewares:   rateLimited(api, deps.RateLimiter, deps.Logger, signupPolicy),
	}, handlers.register)
}

func (h *signupHandlers) register(ctx context.Context, in *registerPracticeInput) (*registerPracticeOutput, error) {
	result, err := h.signup.Execute(ctx, signupapp.Input{
		PracticeName: in.Body.PracticeName,
		Slug:         in.Body.Slug,
		OwnerName:    in.Body.OwnerName,
		OwnerEmail:   in.Body.OwnerEmail,
		Password:     in.Body.Password,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &registerPracticeOutput{Location: APIPrefix + "/tenants/" + result.Slug}
	out.Body = signupBody{
		Tenant: tenantBody{
			ID:          result.TenantID,
			Slug:        result.Slug,
			Name:        result.Name,
			Status:      result.Status,
			Timezone:    result.Timezone,
			CreatedAt:   result.CreatedAt.UTC().Format(time.RFC3339),
			TrialEndsAt: result.TrialEndsAt.UTC().Format(time.RFC3339),
		},
		Owner: signupOwnerBody{
			ID:            result.OwnerID,
			Email:         result.OwnerEmail,
			Role:          result.OwnerRole,
			EmailVerified: false,
		},
	}
	return out, nil
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type registerPracticeInput struct {
	Body registerPracticeBody
}

type registerPracticeBody struct {
	PracticeName string `json:"practiceName" minLength:"1" maxLength:"120" example:"Consultorio Sur" doc:"Display name of the practice."`
	Slug         string `json:"slug" minLength:"3" maxLength:"63" pattern:"^[a-z0-9]+(-[a-z0-9]+)*$" example:"consultorio-sur" doc:"Lowercase DNS label: letters, digits and single inner hyphens. Becomes the practice's subdomain."`
	OwnerName    string `json:"ownerName" minLength:"1" maxLength:"200" example:"Ana Pérez" doc:"Name of the owner account."`
	OwnerEmail   string `json:"ownerEmail" minLength:"3" maxLength:"254" example:"ana@consultoriosur.test" doc:"Email the owner signs in with."`
	Password     string `json:"password" doc:"At least 12 characters, at most 256."`
}

type registerPracticeOutput struct {
	Location string `header:"Location" doc:"URL of the created practice."`
	Body     signupBody
}

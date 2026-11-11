package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	authapp "github.com/niagga/aveniq/backend/internal/application/auth"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// loginBody is what a client posts to start a session. The tenant is explicit
// because the email is unique per practice, not platform-wide.
type loginBody struct {
	TenantSlug string `json:"tenantSlug" minLength:"1" maxLength:"63" example:"consultorio-sur" doc:"Slug of the practice the account belongs to."`
	Email      string `json:"email" minLength:"3" maxLength:"320" example:"ana@consultorio.test" doc:"Login address."`
	Password   string `json:"password" minLength:"1" maxLength:"1024" doc:"Plain password; only ever sent over TLS."`
}

// refreshBody carries the token to rotate or revoke.
type refreshBody struct {
	RefreshToken string `json:"refreshToken" minLength:"1" doc:"Opaque refresh token from the previous login or rotation."`
}

// sessionIdentity is the identity the issued tokens carry. It is repeated in the
// body so a client can render the session without decoding the access token.
type sessionIdentity struct {
	ID       string `json:"id" format:"uuid" doc:"User identifier."`
	TenantID string `json:"tenantId" format:"uuid" doc:"Tenant the user belongs to."`
	Role     string `json:"role" doc:"Role code of the user inside the tenant."`
}

// sessionBody is the token pair. The refresh token lives 30 days and rotates on
// every use; storing it is the client's problem, and a browser client should keep
// it out of JavaScript-reachable storage.
type sessionBody struct {
	AccessToken      string          `json:"accessToken" doc:"Short-lived bearer token."`
	TokenType        string          `json:"tokenType" enum:"Bearer" doc:"Always Bearer."`
	ExpiresIn        int             `json:"expiresIn" doc:"Access token lifetime, in seconds."`
	RefreshToken     string          `json:"refreshToken" doc:"Rotate it on every use; reusing a spent one revokes the session chain."`
	RefreshExpiresAt string          `json:"refreshExpiresAt" format:"date-time" doc:"Expiry of the refresh grant, RFC 3339 in UTC."`
	User             sessionIdentity `json:"user"`
}

type loginInput struct {
	Body loginBody
}

type loginOutput struct {
	Body sessionBody
}

type refreshInput struct {
	Body refreshBody
}

type refreshOutput struct {
	Body sessionBody
}

type logoutInput struct {
	Body refreshBody
}

// meOutput is the identity resolved from the bearer token, with nothing read from
// the database: it is the proof that the token is live and well signed.
type meOutput struct {
	Body sessionIdentity
}

// ─── handlers ────────────────────────────────────────────────────────────────

type authHandlers struct {
	login   *authapp.LoginUseCase
	refresh *authapp.RefreshUseCase
	logout  *authapp.LogoutUseCase
	logger  *slog.Logger
}

// registerAuthRoutes declares the authentication endpoints. Only `me` is
// authenticated here; which tenant operations require an identity is T15's
// decision, and wiring it early would guess at a policy that does not exist yet.
func registerAuthRoutes(api huma.API, deps Deps) {
	handlers := &authHandlers{
		login:   deps.Login,
		refresh: deps.Refresh,
		logout:  deps.Logout,
		logger:  deps.Logger,
	}

	huma.Register(api, huma.Operation{
		OperationID:   "login",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/auth/login",
		Summary:       "Start a session",
		Description:   "Exchanges a tenant slug, an email and a password for an access token and a refresh grant. Unknown tenant, unknown email, inactive user and wrong password are answered identically.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusOK,
	}, handlers.loginHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "refresh-session",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/auth/refresh",
		Summary:       "Rotate the session",
		Description:   "Spends the presented refresh token and returns a new pair. Presenting an already spent token revokes the whole session chain.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusOK,
	}, handlers.refreshHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "logout",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/auth/logout",
		Summary:       "End a session",
		Description:   "Revokes the presented refresh grant. It answers 204 even for an unknown or already revoked token, so it cannot be used to test whether a token exists.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusNoContent,
	}, handlers.logoutHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "whoami",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/auth/me",
		Summary:       "Describe the current identity",
		Description:   "Returns the identity carried by the access token.",
		Tags:          []string{"auth"},
		DefaultStatus: http.StatusOK,
		Security:      []map[string][]string{{securitySchemeBearer: {}}},
		Middlewares:   huma.Middlewares{authenticate(api, deps.AccessTokens)},
	}, handlers.meHandler)
}

func (h *authHandlers) loginHandler(ctx context.Context, in *loginInput) (*loginOutput, error) {
	session, err := h.login.Execute(ctx, authapp.LoginInput{
		TenantSlug: in.Body.TenantSlug,
		Email:      in.Body.Email,
		Password:   in.Body.Password,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &loginOutput{Body: toSessionBody(session)}, nil
}

func (h *authHandlers) refreshHandler(ctx context.Context, in *refreshInput) (*refreshOutput, error) {
	session, err := h.refresh.Execute(ctx, authapp.RefreshInput{RefreshToken: in.Body.RefreshToken})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &refreshOutput{Body: toSessionBody(session)}, nil
}

// logoutHandler answers 204 whatever happened, except for a storage failure: the
// client can only act on "you are logged out".
func (h *authHandlers) logoutHandler(ctx context.Context, in *logoutInput) (*struct{}, error) {
	if err := h.logout.Execute(ctx, authapp.LogoutInput{RefreshToken: in.Body.RefreshToken}); err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return nil, nil
}

func (h *authHandlers) meHandler(ctx context.Context, _ *struct{}) (*meOutput, error) {
	identity, ok := IdentityFrom(ctx)
	if !ok {
		// The middleware runs first; reaching here without an identity means the
		// route was registered without it, which is a wiring defect, not a request.
		h.logger.ErrorContext(ctx, "authenticated route reached without an identity")
		return nil, huma.Error500InternalServerError("Unexpected error")
	}

	out := &meOutput{}
	out.Body = sessionIdentity{ID: identity.UserID, TenantID: identity.TenantID, Role: identity.Role}
	return out, nil
}

// toSessionBody maps the application result onto the wire contract.
func toSessionBody(session authapp.Session) sessionBody {
	return sessionBody{
		AccessToken:      session.AccessToken,
		TokenType:        "Bearer",
		ExpiresIn:        int(session.ExpiresIn / time.Second),
		RefreshToken:     session.RefreshToken,
		RefreshExpiresAt: session.RefreshExpiresAt.UTC().Format(time.RFC3339),
		User: sessionIdentity{
			ID:       session.UserID,
			TenantID: session.TenantID,
			Role:     session.Role,
		},
	}
}

package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	invitationapp "github.com/niagga/aveniq/backend/internal/application/invitation"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// invitationBody is a pending invitation as the administrative screen sees it. The
// token and its hash never appear here: the token is shown once, at creation.
type invitationBody struct {
	ID        string `json:"id" format:"uuid" doc:"Invitation identifier."`
	Email     string `json:"email" doc:"Canonical email the account will be created with."`
	Role      string `json:"role" enum:"owner,admin,professional,assistant,accountant" doc:"The role the account will hold."`
	InvitedBy string `json:"invitedBy,omitempty" format:"uuid" doc:"Account that issued it. Empty once that account was removed."`
	ExpiresAt string `json:"expiresAt" format:"date-time"`
	CreatedAt string `json:"createdAt" format:"date-time"`
}

// issuedInvitationBody is the creation answer. Token is the link secret; the
// screen builds the link from it and shows it once to the inviter.
type issuedInvitationBody struct {
	Invitation invitationBody `json:"invitation"`
	Token      string         `json:"token" doc:"Link secret. Shown once; the server keeps only its hash."`
}

// previewBody is what the public link page may show before the invitee acts.
type previewBody struct {
	Email     string `json:"email" doc:"Email the account will be created with."`
	Role      string `json:"role" enum:"owner,admin,professional,assistant,accountant" doc:"Role the invitee will hold."`
	ExpiresAt string `json:"expiresAt" format:"date-time"`
}

// acceptedBody is the account created by an acceptance. It carries no session: the
// invitee signs in with the password they just chose.
type acceptedBody struct {
	UserID string `json:"userId" format:"uuid"`
	Email  string `json:"email"`
	Role   string `json:"role" enum:"owner,admin,professional,assistant,accountant"`
}

func toInvitationBody(view invitationapp.InvitationView) invitationBody {
	return invitationBody{
		ID:        view.ID,
		Email:     view.Email,
		Role:      view.Role,
		InvitedBy: view.InvitedBy,
		ExpiresAt: view.ExpiresAt.UTC().Format(time.RFC3339),
		CreatedAt: view.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ─── handlers ────────────────────────────────────────────────────────────────

type invitationHandlers struct {
	create  *invitationapp.CreateUseCase
	list    *invitationapp.ListPendingUseCase
	revoke  *invitationapp.RevokeUseCase
	preview *invitationapp.PreviewUseCase
	accept  *invitationapp.AcceptUseCase
	logger  *slog.Logger
}

// registerInvitationRoutes declares the invitation endpoints.
//
// Issuing, listing and revoking need users:manage: the same permission that changes
// accounts, so whoever can change a role can also offer one. The link page is
// public: the preview and the acceptance authenticate with the token itself, and
// they reveal nothing about the practice beyond the invitation.
//
// The public routes share one rate limit per client (invitationLinkPolicy), so a
// guessed secret cannot be tried at speed.
func registerInvitationRoutes(api huma.API, deps Deps) {
	handlers := &invitationHandlers{
		create:  deps.CreateInvitation,
		list:    deps.ListPendingInvitations,
		revoke:  deps.RevokeInvitation,
		preview: deps.PreviewInvitation,
		accept:  deps.AcceptInvitation,
		logger:  deps.Logger,
	}

	manageGuard := permissionGuard(api, deps.AccessTokens, iam.PermUsersManage)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "create-invitation",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/invitations",
		Summary:       "Offer an account in the practice",
		Description:   "Creates a single-use link valid for seven days. The token in the answer is the only copy: the screen shows it to the inviter once. Only an owner offers the owner role. Answers 409 when the email already has an account in the practice.",
		Tags:          []string{"invitations"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   manageGuard,
	}, handlers.createInvitation)

	huma.Register(api, huma.Operation{
		OperationID:   "list-pending-invitations",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/invitations",
		Summary:       "List pending invitations",
		Description:   "Invitations not yet accepted and not expired, newest first. Accepted and expired ones are not listed.",
		Tags:          []string{"invitations"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   manageGuard,
	}, handlers.listInvitations)

	huma.Register(api, huma.Operation{
		OperationID:   "revoke-invitation",
		Method:        http.MethodDelete,
		Path:          APIPrefix + "/invitations/{id}",
		Summary:       "Revoke a pending invitation",
		Description:   "The link stops working at once. An accepted invitation cannot be revoked (409).",
		Tags:          []string{"invitations"},
		DefaultStatus: http.StatusNoContent,
		Security:      secured,
		Middlewares:   manageGuard,
	}, handlers.revokeInvitation)

	huma.Register(api, huma.Operation{
		OperationID:   "preview-invitation",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/invitations/preview",
		Summary:       "Read an invitation from its link",
		Description:   "Public. Answers 410 for any link that does not open an account: malformed, unknown, accepted or expired. The same answer for all four, on purpose.",
		Tags:          []string{"invitations"},
		DefaultStatus: http.StatusOK,
		Middlewares:   rateLimited(api, deps.RateLimiter, deps.Logger, invitationLinkPolicy),
	}, handlers.previewInvitation)

	huma.Register(api, huma.Operation{
		OperationID:   "accept-invitation",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/invitations/accept",
		Summary:       "Accept an invitation and create the account",
		Description:   "Public. Creates the account with the email and role of the invitation and the password the invitee chooses, then marks the link used. Answers 410 for an unusable link and 422 for a password shorter than 12 characters.",
		Tags:          []string{"invitations"},
		DefaultStatus: http.StatusCreated,
		Middlewares:   rateLimited(api, deps.RateLimiter, deps.Logger, invitationLinkPolicy),
	}, handlers.acceptInvitation)
}

// invitationCallerOf is the identity the guard stored, shaped for the invitation use
// cases. The guard has already run, so the identity is present.
func invitationCallerOf(ctx context.Context) invitationapp.Caller {
	identity := identityOf(ctx)
	return invitationapp.Caller{TenantID: identity.TenantID, UserID: identity.UserID, Role: identity.Role}
}

func (h *invitationHandlers) createInvitation(ctx context.Context, in *createInvitationInput) (*issuedInvitationOutput, error) {
	issued, err := h.create.Execute(ctx, invitationapp.CreateInput{
		Caller: invitationCallerOf(ctx),
		Email:  in.Body.Email,
		Role:   in.Body.Role,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &issuedInvitationOutput{}
	out.Body.Invitation = toInvitationBody(issued.Invitation)
	out.Body.Token = issued.Token
	return out, nil
}

func (h *invitationHandlers) listInvitations(ctx context.Context, in *listInvitationsInput) (*listInvitationsOutput, error) {
	views, err := h.list.Execute(ctx, invitationapp.ListInput{
		Caller: invitationCallerOf(ctx),
		Limit:  in.Limit,
		Offset: in.Offset,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listInvitationsOutput{}
	out.Body.Items = make([]invitationBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toInvitationBody(view))
	}
	return out, nil
}

func (h *invitationHandlers) revokeInvitation(ctx context.Context, in *invitationIDInput) (*struct{}, error) {
	if err := h.revoke.Execute(ctx, invitationCallerOf(ctx), in.ID); err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &struct{}{}, nil
}

func (h *invitationHandlers) previewInvitation(ctx context.Context, in *invitationTokenQuery) (*previewOutput, error) {
	preview, err := h.preview.Execute(ctx, in.Token)
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &previewOutput{}
	out.Body = previewBody{
		Email:     preview.Email,
		Role:      preview.Role,
		ExpiresAt: preview.ExpiresAt.UTC().Format(time.RFC3339),
	}
	return out, nil
}

func (h *invitationHandlers) acceptInvitation(ctx context.Context, in *acceptInvitationInput) (*acceptedOutput, error) {
	accepted, err := h.accept.Execute(ctx, invitationapp.AcceptInput{
		Token:    in.Body.Token,
		FullName: in.Body.FullName,
		Password: in.Body.Password,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &acceptedOutput{}
	out.Body = acceptedBody{
		UserID: accepted.UserID,
		Email:  accepted.Email,
		Role:   accepted.Role,
	}
	return out, nil
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type createInvitationInput struct {
	Body createInvitationBody
}

type createInvitationBody struct {
	Email string `json:"email" doc:"Email the invitee will sign in with. Must not already have an account in the practice."`
	Role  string `json:"role" enum:"owner,admin,professional,assistant,accountant" doc:"Role the account will hold."`
}

type issuedInvitationOutput struct {
	Body issuedInvitationBody
}

type listInvitationsInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size."`
	Offset int32 `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
}

type listInvitationsOutput struct {
	Body struct {
		Items []invitationBody `json:"items"`
	}
}

type invitationIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Invitation identifier."`
}

type invitationTokenQuery struct {
	Token string `query:"token" minLength:"1" doc:"Link secret from the invitation URL."`
}

type previewOutput struct {
	Body previewBody
}

type acceptInvitationInput struct {
	Body acceptInvitationBody
}

type acceptInvitationBody struct {
	Token    string `json:"token" minLength:"1" doc:"Link secret from the invitation URL."`
	FullName string `json:"fullName" minLength:"1" maxLength:"200" doc:"Name the account will show."`
	Password string `json:"password" doc:"Chosen by the invitee. At least 12 characters, at most 256."`
}

type acceptedOutput struct {
	Body acceptedBody
}

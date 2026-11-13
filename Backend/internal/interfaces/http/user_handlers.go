package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	userapp "github.com/niagga/aveniq/backend/internal/application/user"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── wire types ──────────────────────────────────────────────────────────────

// userBody is an account as the administrative screens see it. The password hash
// is not part of any representation.
type userBody struct {
	ID        string `json:"id" format:"uuid" doc:"Account identifier."`
	Email     string `json:"email" doc:"Canonical login email, unique within the practice."`
	FullName  string `json:"fullName" doc:"Name as the practice registered it."`
	Role      string `json:"role" enum:"owner,admin,professional,assistant,accountant" doc:"The one role this account holds in the practice."`
	IsActive  bool   `json:"isActive" doc:"False after a deactivation. Accounts are never deleted."`
	CreatedAt string `json:"createdAt" format:"date-time"`
	UpdatedAt string `json:"updatedAt" format:"date-time"`
}

// directoryEntryBody is the narrow view a clinical entry needs to name its author:
// who, and in what role. No email, no status.
type directoryEntryBody struct {
	ID       string `json:"id" format:"uuid" doc:"Account identifier, the author of a clinical entry."`
	FullName string `json:"fullName" doc:"Name to show next to an entry."`
	Role     string `json:"role" enum:"owner,admin,professional,assistant,accountant"`
}

func toUserBody(view userapp.UserView) userBody {
	return userBody{
		ID:        view.ID,
		Email:     view.Email,
		FullName:  view.FullName,
		Role:      view.Role,
		IsActive:  view.IsActive,
		CreatedAt: view.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: view.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// ─── handlers ────────────────────────────────────────────────────────────────

type userHandlers struct {
	list       *userapp.ListUseCase
	get        *userapp.GetUseCase
	changeRole *userapp.ChangeRoleUseCase
	deactivate *userapp.DeactivateUseCase
	activate   *userapp.ActivateUseCase
	logger     *slog.Logger
}

// registerUserRoutes declares the account and directory endpoints.
//
// Every route runs under a permission from the matrix (T33). Reading the full
// account needs users:read, which only owner and admin hold. Naming an author needs
// users:directory, which the clinical roles hold too. Changing an account needs
// users:manage, and the rules about who may change whom are in iam.AuthorizeChange,
// not here.
//
// There is no POST: an account is created by accepting an invitation (T35), because
// the row needs a password hash that only the invitee can set.
func registerUserRoutes(api huma.API, deps Deps) {
	handlers := &userHandlers{
		list:       deps.ListUsers,
		get:        deps.GetUser,
		changeRole: deps.ChangeUserRole,
		deactivate: deps.DeactivateUser,
		activate:   deps.ActivateUser,
		logger:     deps.Logger,
	}

	readGuard := permissionGuard(api, deps.AccessTokens, iam.PermUsersRead)
	directoryGuard := permissionGuard(api, deps.AccessTokens, iam.PermUsersDirectory)
	manageGuard := permissionGuard(api, deps.AccessTokens, iam.PermUsersManage)
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-users",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/users",
		Summary:       "List the accounts of the practice",
		Description:   "One page of accounts, newest first, including deactivated ones. Needs users:read.",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.listUsers)

	huma.Register(api, huma.Operation{
		OperationID:   "list-staff-directory",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/users/directory",
		Summary:       "Name the staff of the practice",
		Description:   "The same page as the account list, reduced to id, name and role. Used to show who wrote a clinical entry. No email, no status.",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   directoryGuard,
	}, handlers.staffDirectory)

	huma.Register(api, huma.Operation{
		OperationID:   "get-user",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/users/{id}",
		Summary:       "Get an account",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   readGuard,
	}, handlers.getUser)

	huma.Register(api, huma.Operation{
		OperationID:   "change-user-role",
		Method:        http.MethodPut,
		Path:          APIPrefix + "/users/{id}/role",
		Summary:       "Change the role of an account",
		Description:   "Nobody changes their own role. Only an owner grants or changes the owner role. Answers 409 when the account already has that role.",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   manageGuard,
	}, handlers.changeUserRole)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivate-user",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/users/{id}/deactivate",
		Summary:       "Deactivate an account",
		Description:   "Blocks the account from signing in without deleting it. Its clinical entries keep their author.",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   manageGuard,
	}, handlers.deactivateUser)

	huma.Register(api, huma.Operation{
		OperationID:   "activate-user",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/users/{id}/activate",
		Summary:       "Reactivate an account",
		Tags:          []string{"users"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   manageGuard,
	}, handlers.activateUser)
}

func (h *userHandlers) listUsers(ctx context.Context, in *listUsersInput) (*listUsersOutput, error) {
	views, err := h.list.Execute(ctx, userapp.ListInput{
		Caller: callerOf(ctx),
		Limit:  in.Limit,
		Offset: in.Offset,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listUsersOutput{}
	out.Body.Items = make([]userBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toUserBody(view))
	}
	return out, nil
}

func (h *userHandlers) staffDirectory(ctx context.Context, in *listUsersInput) (*staffDirectoryOutput, error) {
	views, err := h.list.Execute(ctx, userapp.ListInput{
		Caller: callerOf(ctx),
		Limit:  in.Limit,
		Offset: in.Offset,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &staffDirectoryOutput{}
	out.Body.Items = make([]directoryEntryBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, directoryEntryBody{
			ID:       view.ID,
			FullName: view.FullName,
			Role:     view.Role,
		})
	}
	return out, nil
}

func (h *userHandlers) getUser(ctx context.Context, in *userIDInput) (*userOutput, error) {
	view, err := h.get.Execute(ctx, identityOf(ctx).TenantID, in.ID)
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &userOutput{Body: toUserBody(view)}, nil
}

func (h *userHandlers) changeUserRole(ctx context.Context, in *changeRoleInput) (*userOutput, error) {
	view, err := h.changeRole.Execute(ctx, userapp.ChangeRoleInput{
		Caller:   callerOf(ctx),
		TargetID: in.ID,
		Role:     in.Body.Role,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &userOutput{Body: toUserBody(view)}, nil
}

func (h *userHandlers) deactivateUser(ctx context.Context, in *userIDInput) (*userOutput, error) {
	view, err := h.deactivate.Execute(ctx, callerOf(ctx), in.ID)
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &userOutput{Body: toUserBody(view)}, nil
}

func (h *userHandlers) activateUser(ctx context.Context, in *userIDInput) (*userOutput, error) {
	view, err := h.activate.Execute(ctx, callerOf(ctx), in.ID)
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return &userOutput{Body: toUserBody(view)}, nil
}

// callerOf is the identity the guard stored, shaped for the use cases.
func callerOf(ctx context.Context) userapp.Caller {
	identity := identityOf(ctx)
	return userapp.Caller{TenantID: identity.TenantID, UserID: identity.UserID, Role: identity.Role}
}

// ─── inputs and outputs ──────────────────────────────────────────────────────

type listUsersInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Page size."`
	Offset int32 `query:"offset" minimum:"0" default:"0" doc:"Rows to skip."`
}

type listUsersOutput struct {
	Body struct {
		Items []userBody `json:"items"`
	}
}

type staffDirectoryOutput struct {
	Body struct {
		Items []directoryEntryBody `json:"items"`
	}
}

type userIDInput struct {
	ID string `path:"id" format:"uuid" doc:"Account identifier."`
}

type changeRoleInput struct {
	ID   string `path:"id" format:"uuid" doc:"Account identifier."`
	Body changeRoleBody
}

type changeRoleBody struct {
	Role string `json:"role" enum:"owner,admin,professional,assistant,accountant" doc:"The role the account will hold."`
}

type userOutput struct {
	Body userBody
}

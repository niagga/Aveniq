package userapp

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// Pagination bounds, same policy as the patient roster: decided here, not in SQL.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// ─── reads ───────────────────────────────────────────────────────────────────

// ListInput requests one page of the practice's accounts.
type ListInput struct {
	Caller
	Limit  int32
	Offset int32
}

// ListUseCase lists accounts of the caller's practice, newest first.
type ListUseCase struct {
	deps
}

// NewListUseCase wires the use case.
func NewListUseCase(users port.UserRepository, scope port.TenantScope) *ListUseCase {
	return &ListUseCase{deps{users: users, scope: scope}}
}

// Execute returns one page. A zero limit is the default page; an out-of-range one
// is refused here, before the database sees it.
func (uc *ListUseCase) Execute(ctx context.Context, in ListInput) ([]UserView, error) {
	limit := in.Limit
	if limit == 0 {
		limit = DefaultPageSize
	}
	if limit < 0 || limit > MaxPageSize {
		return nil, &iam.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	if in.Offset < 0 {
		return nil, &iam.ValidationError{Field: "offset", Message: "must not be negative"}
	}

	var views []UserView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		rows, err := uc.users.List(ctx, limit, in.Offset)
		if err != nil {
			return err
		}
		views = make([]UserView, 0, len(rows))
		for _, row := range rows {
			views = append(views, toView(row))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// GetUseCase resolves one account of the caller's practice.
type GetUseCase struct {
	deps
}

// NewGetUseCase wires the use case.
func NewGetUseCase(users port.UserRepository, scope port.TenantScope) *GetUseCase {
	return &GetUseCase{deps{users: users, scope: scope}}
}

// Execute returns the account or iam.ErrNotFound. A foreign account is not found.
func (uc *GetUseCase) Execute(ctx context.Context, tenantID, id string) (UserView, error) {
	var view UserView
	err := uc.scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		row, err := uc.users.ByID(ctx, iam.UserID(id))
		if err != nil {
			return err
		}
		view = toView(row)
		return nil
	})
	if err != nil {
		return UserView{}, err
	}
	return view, nil
}

// ─── changes ─────────────────────────────────────────────────────────────────

// ChangeRoleInput moves an account to another role.
type ChangeRoleInput struct {
	Caller
	TargetID string
	Role     string
}

// ChangeRoleUseCase changes the role of an account in the caller's practice.
type ChangeRoleUseCase struct {
	deps
}

// NewChangeRoleUseCase wires the use case.
func NewChangeRoleUseCase(users port.UserRepository, scope port.TenantScope, audit port.AuditLog, clock port.Clock) *ChangeRoleUseCase {
	return &ChangeRoleUseCase{newDeps(users, scope, audit, clock)}
}

// Execute applies the change or returns the domain error untouched: unknown role
// (validation), same role (invalid transition), a forbidden change (not allowed),
// or a foreign account (not found).
func (uc *ChangeRoleUseCase) Execute(ctx context.Context, in ChangeRoleInput) (UserView, error) {
	role := iam.RoleCode(in.Role)
	if !role.Valid() {
		return UserView{}, &iam.ValidationError{Field: "role", Message: "must be one of the seeded roles"}
	}

	return uc.runChange(ctx, in.Caller, in.TargetID, func(current iam.User, now time.Time) (change, error) {
		if err := iam.AuthorizeChange(actorOf(in.Caller), current, role); err != nil {
			return change{}, err
		}
		if current.Role == role {
			return change{}, iam.ErrInvalidTransition
		}
		next, err := current.ChangeRole(role, now)
		if err != nil {
			return change{}, err
		}
		return change{
			action:   "user.role_changed",
			metadata: map[string]any{"from": string(current.Role), "to": string(role)},
			next:     next,
		}, nil
	})
}

// DeactivateUseCase blocks an account without deleting it. Its audit trail and any
// clinical entries it authored keep their author.
type DeactivateUseCase struct {
	deps
}

// NewDeactivateUseCase wires the use case.
func NewDeactivateUseCase(users port.UserRepository, scope port.TenantScope, audit port.AuditLog, clock port.Clock) *DeactivateUseCase {
	return &DeactivateUseCase{newDeps(users, scope, audit, clock)}
}

// Execute deactivates the account. An already inactive one is an invalid transition.
func (uc *DeactivateUseCase) Execute(ctx context.Context, who Caller, targetID string) (UserView, error) {
	return uc.runChange(ctx, who, targetID, func(current iam.User, now time.Time) (change, error) {
		if err := iam.AuthorizeChange(actorOf(who), current, current.Role); err != nil {
			return change{}, err
		}
		next, err := current.Deactivate(now)
		if err != nil {
			return change{}, err
		}
		return change{action: "user.deactivated", next: next}, nil
	})
}

// ActivateUseCase restores a deactivated account.
type ActivateUseCase struct {
	deps
}

// NewActivateUseCase wires the use case.
func NewActivateUseCase(users port.UserRepository, scope port.TenantScope, audit port.AuditLog, clock port.Clock) *ActivateUseCase {
	return &ActivateUseCase{newDeps(users, scope, audit, clock)}
}

// Execute reactivates the account. An already active one is an invalid transition.
func (uc *ActivateUseCase) Execute(ctx context.Context, who Caller, targetID string) (UserView, error) {
	return uc.runChange(ctx, who, targetID, func(current iam.User, now time.Time) (change, error) {
		if err := iam.AuthorizeChange(actorOf(who), current, current.Role); err != nil {
			return change{}, err
		}
		next, err := current.Activate(now)
		if err != nil {
			return change{}, err
		}
		return change{action: "user.activated", next: next}, nil
	})
}

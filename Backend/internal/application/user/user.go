// Package userapp is the use cases for the people who work in a practice: read
// the accounts, read the staff directory, and change what an account may do
// (role, deactivation, reactivation). Creating an account is the invitation flow
// (T35): a user row needs a password hash, and only the invitee can set one.
package userapp

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// Caller is who asks: the identity the verified access token carries. The tenant
// scopes the transaction; the actor is both the authorization subject and the
// author of the audit entry.
type Caller struct {
	TenantID string
	UserID   string
	Role     string
}

// UserView is an account as the administrative screens read it. It never carries
// the password hash, not even its presence.
type UserView struct {
	ID        string
	Email     string
	FullName  string
	Role      string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func toView(user iam.User) UserView {
	return UserView{
		ID:        string(user.ID),
		Email:     user.Email,
		FullName:  user.FullName,
		Role:      string(user.Role),
		IsActive:  user.IsActive,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

// deps is what every use case in this package needs. Grouped because the change
// flow touches all four, and passing them one by one invites a missing collaborator.
type deps struct {
	users port.UserRepository
	scope port.TenantScope
	audit port.AuditLog
	clock port.Clock
}

// newDeps is shared by the constructors below.
func newDeps(users port.UserRepository, scope port.TenantScope, audit port.AuditLog, clock port.Clock) deps {
	return deps{users: users, scope: scope, audit: audit, clock: clock}
}

// change is one planned mutation, computed from the account as it is stored now.
type change struct {
	action   string
	metadata map[string]any
	next     iam.User
}

// runChange is the one path every write takes: load the target inside the caller's
// tenant scope, plan the change (which is where authorization happens), persist it,
// and record the audit entry in the same transaction. An audit failure rolls the
// change back: a role change nobody can trace is not a change that happened.
func (d deps) runChange(ctx context.Context, who Caller, targetID string, plan func(current iam.User, now time.Time) (change, error)) (UserView, error) {
	var view UserView
	err := d.scope.WithTenant(ctx, who.TenantID, func(ctx context.Context) error {
		current, err := d.users.ByID(ctx, iam.UserID(targetID))
		if err != nil {
			return err
		}
		planned, err := plan(current, d.clock.Now())
		if err != nil {
			return err
		}
		if err := d.users.Update(ctx, planned.next); err != nil {
			return err
		}
		if err := d.audit.Record(ctx, port.AuditEvent{
			TenantID: who.TenantID,
			ActorID:  who.UserID,
			Action:   planned.action,
			Entity:   "user",
			EntityID: targetID,
			Metadata: planned.metadata,
		}); err != nil {
			return err
		}
		view = toView(planned.next)
		return nil
	})
	if err != nil {
		return UserView{}, err
	}
	return view, nil
}

func actorOf(who Caller) iam.Actor {
	return iam.Actor{ID: iam.UserID(who.UserID), Role: iam.RoleCode(who.Role)}
}

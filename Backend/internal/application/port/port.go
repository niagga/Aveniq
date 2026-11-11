// Package port declares the boundaries the application layer needs from the
// outside world. Interfaces live with their consumer (not with the implementation)
// so that infrastructure depends on application, never the other way around.
//
// This is the Go form of the dependency-inversion principle: the use cases know
// only these contracts, and every adapter — Postgres, Redis, a fake for tests —
// is interchangeable behind them.
package port

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// UserRepository persists and reads the IAM user aggregate.
//
// Implementations must map their own failures onto the domain sentinels
// (iam.ErrNotFound, iam.ErrEmailTaken) so the application layer never has to
// recognise a driver-specific error. Every call runs inside a tenant scope:
// see TenantScope.
type UserRepository interface {
	Create(ctx context.Context, subject iam.User) error
	ByID(ctx context.Context, id iam.UserID) (iam.User, error)
	ByEmail(ctx context.Context, email string) (iam.User, error)
	List(ctx context.Context, limit, offset int32) ([]iam.User, error)
	Update(ctx context.Context, subject iam.User) error
	Delete(ctx context.Context, id iam.UserID) error
}

// TenantScope runs a unit of work inside a transaction that carries the tenant
// context row-level security reads (app.tenant_id). It is the transactional
// boundary the IAM repositories assume: decision 4 of L6 — the context is set
// per transaction with the transactional form of SET LOCAL, never per
// connection, because pooled connections are reused across tenants.
type TenantScope interface {
	WithTenant(ctx context.Context, tenantID string, fn func(ctx context.Context) error) error
}

// TenantRepository persists and reads the tenant aggregate.
//
// Implementations must map their own failures onto the domain sentinels
// (tenant.ErrNotFound, tenant.ErrSlugTaken) so the application layer never has
// to recognise a driver-specific error.
type TenantRepository interface {
	Create(ctx context.Context, subject tenant.Tenant) error
	BySlug(ctx context.Context, slug string) (tenant.Tenant, error)
	List(ctx context.Context, limit, offset int32) ([]tenant.Tenant, error)
}

// Clock abstracts time so use cases are deterministic under test.
type Clock interface {
	Now() time.Time
}

// IDGenerator abstracts identity generation for the same reason.
type IDGenerator interface {
	NewID() string
}

// SessionRepository persists the refresh sessions of the IAM aggregate.
//
// Decision 7 of L11 lives in the implementation: a foreign key inside `sessions`
// does not go through row-level security, so building a session on a user UUID
// from another tenant must be refused by the adapter, not by a caller that could
// forget. Every call runs inside a tenant scope (see TenantScope).
type SessionRepository interface {
	Create(ctx context.Context, subject iam.Session) error
	ByRefreshHash(ctx context.Context, hash iam.RefreshHash) (iam.Session, error)
	Rotate(ctx context.Context, previous, successor iam.Session) error
	Revoke(ctx context.Context, id iam.SessionID, at time.Time) error
	RevokeChain(ctx context.Context, id iam.SessionID, at time.Time) (int64, error)
}

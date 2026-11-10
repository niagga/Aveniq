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

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

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

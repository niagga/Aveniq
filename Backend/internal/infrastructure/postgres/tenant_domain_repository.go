package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// TenantDomainRepository is the PostgreSQL adapter for port.TenantDomainRepository.
//
// The host map has no row-level security and no tenant scope: the public landing asks it
// which practice a host belongs to before any tenant context exists. It reads through the pool.
type TenantDomainRepository struct {
	queries *sqlcgen.Queries
}

// NewTenantDomainRepository builds the adapter over an existing pool.
func NewTenantDomainRepository(pool *pgxpool.Pool) *TenantDomainRepository {
	return &TenantDomainRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.TenantDomainRepository = (*TenantDomainRepository)(nil)

// TenantIDByHost returns the practice that answers on the host, or tenant.ErrNotFound.
func (r *TenantDomainRepository) TenantIDByHost(ctx context.Context, host string) (tenant.ID, error) {
	id, err := r.queries.GetTenantIDByHost(ctx, host)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", tenant.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return tenantIDOf(id), nil
}

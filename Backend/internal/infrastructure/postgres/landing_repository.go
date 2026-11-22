package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/landing"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// LandingRepository is the PostgreSQL adapter for port.LandingRepository.
//
// Like the insurer repository it requires Transactor.WithTenant: row-level security derives
// the practice from app.tenant_id, so the queries carry no tenant filter.
type LandingRepository struct {
	queries *sqlcgen.Queries
}

// NewLandingRepository builds the adapter over an existing pool.
func NewLandingRepository(pool *pgxpool.Pool) *LandingRepository {
	return &LandingRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.LandingRepository = (*LandingRepository)(nil)

// scoped binds the queries to the open tenant transaction, failing instead of falling back to
// the pool: a read on the pool would see no rows under RLS and pass for "not written yet".
func (r *LandingRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, landing.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Get reads the landing of the practice in scope, or landing.ErrNotFound when it has none yet.
func (r *LandingRepository) Get(ctx context.Context) (landing.Profile, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return landing.Profile{}, err
	}

	row, err := queries.GetTenantProfile(ctx)
	if err != nil {
		return landing.Profile{}, translateLandingError(err)
	}
	return landingToDomain(row)
}

// Save writes the landing of the practice in scope, creating it on first save.
func (r *LandingRepository) Save(ctx context.Context, subject landing.Profile) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}

	_, err = queries.UpsertTenantProfile(ctx, sqlcgen.UpsertTenantProfileParams{
		TenantID:    tenantID,
		DisplayName: subject.DisplayName,
		Tagline:     subject.Tagline,
		About:       subject.About,
		Address:     subject.Address,
		Phone:       subject.Phone,
		Email:       subject.Email,
		Schedule:    subject.Schedule,
		UpdatedAt:   subject.UpdatedAt,
	})
	return translateLandingError(err)
}

// landingToDomain rebuilds the landing from a row through the validating constructor.
func landingToDomain(row sqlcgen.TenantProfile) (landing.Profile, error) {
	subject, err := landing.Reconstitute(tenantIDOf(row.TenantID), landing.Draft{
		DisplayName: row.DisplayName,
		Tagline:     row.Tagline,
		About:       row.About,
		Address:     row.Address,
		Phone:       row.Phone,
		Email:       row.Email,
		Schedule:    row.Schedule,
	}, row.UpdatedAt)
	if err != nil {
		return landing.Profile{}, fmt.Errorf("postgres: stored landing of %s is invalid: %w", tenantIDOf(row.TenantID), err)
	}
	return subject, nil
}

// translateLandingError maps storage failures onto the landing domain's vocabulary.
func translateLandingError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return landing.ErrNotFound
	}
	return err
}

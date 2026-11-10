package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// uniqueViolationCode is Postgres' SQLSTATE for a unique constraint breach.
const uniqueViolationCode = "23505"

// TenantRepository is the PostgreSQL adapter for port.TenantRepository.
//
// It implements the interface implicitly, which is the point: the application
// layer never mentions this type, so swapping the adapter for another storage
// engine requires no change on the consumer side.
type TenantRepository struct {
	queries *sqlcgen.Queries
}

// NewTenantRepository builds the adapter over an existing pool.
func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository {
	return &TenantRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ interface {
	Create(ctx context.Context, subject tenant.Tenant) error
	BySlug(ctx context.Context, slug string) (tenant.Tenant, error)
	List(ctx context.Context, limit, offset int32) ([]tenant.Tenant, error)
} = (*TenantRepository)(nil)

// Create inserts a new tenant, translating a slug collision into the domain error
// so the application layer never has to recognise a driver-specific code.
func (r *TenantRepository) Create(ctx context.Context, subject tenant.Tenant) error {
	id, err := toPGUUID(subject.ID)
	if err != nil {
		return err
	}

	_, err = r.queries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        id,
		Slug:      subject.Slug,
		Name:      subject.Name,
		Status:    string(subject.Status),
		CreatedAt: subject.CreatedAt,
	})
	if err != nil {
		return translateError(err)
	}
	return nil
}

// BySlug returns the tenant matching the slug, or tenant.ErrNotFound.
func (r *TenantRepository) BySlug(ctx context.Context, slug string) (tenant.Tenant, error) {
	row, err := r.queries.GetTenantBySlug(ctx, slug)
	if err != nil {
		return tenant.Tenant{}, translateError(err)
	}
	return toDomain(row)
}

// List returns one page of tenants, newest first.
func (r *TenantRepository) List(ctx context.Context, limit, offset int32) ([]tenant.Tenant, error) {
	rows, err := r.queries.ListTenants(ctx, sqlcgen.ListTenantsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, translateError(err)
	}

	subjects := make([]tenant.Tenant, 0, len(rows))
	for _, row := range rows {
		subject, err := toDomain(row)
		if err != nil {
			return nil, err
		}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

// toDomain rebuilds the aggregate through its validating constructor.
func toDomain(row sqlcgen.Tenant) (tenant.Tenant, error) {
	subject, err := tenant.Reconstitute(
		tenant.ID(row.ID.String()),
		row.Slug,
		row.Name,
		tenant.Status(row.Status),
		row.CreatedAt,
	)
	if err != nil {
		return tenant.Tenant{}, fmt.Errorf("postgres: stored tenant %s is invalid: %w", row.ID.String(), err)
	}
	return subject, nil
}

// translateError maps storage failures onto the domain's vocabulary.
func translateError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return tenant.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
		return fmt.Errorf("%w: constraint %s", tenant.ErrSlugTaken, pgErr.ConstraintName)
	}

	return err
}

func toPGUUID(id tenant.ID) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("postgres: %q is not a valid UUID: %w", id, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

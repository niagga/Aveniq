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

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// insurersTenantNameUnique is the index behind per-practice name uniqueness.
const insurersTenantNameUnique = "insurers_tenant_name_unique"

// InsurerRepository is the PostgreSQL adapter for port.InsurerRepository.
//
// Like the patient repository it requires Transactor.WithTenant: row-level security
// derives the practice from app.tenant_id, so the queries carry no tenant filter.
type InsurerRepository struct {
	queries *sqlcgen.Queries
}

// NewInsurerRepository builds the adapter over an existing pool.
func NewInsurerRepository(pool *pgxpool.Pool) *InsurerRepository {
	return &InsurerRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.InsurerRepository = (*InsurerRepository)(nil)

// scoped binds the queries to the open tenant transaction, failing instead of falling
// back to the pool, for the same reason the patient adapter does.
func (r *InsurerRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, insurer.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Create inserts a new mutual.
func (r *InsurerRepository) Create(ctx context.Context, subject insurer.Insurer) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}

	_, err = queries.CreateInsurer(ctx, sqlcgen.CreateInsurerParams{
		ID:        toInsurerUUID(subject.ID),
		TenantID:  tenantID,
		Name:      subject.Name,
		TaxID:     subject.TaxID,
		IsActive:  subject.IsActive,
		CreatedAt: subject.CreatedAt,
		UpdatedAt: subject.UpdatedAt,
	})
	if err != nil {
		return translateInsurerError(err)
	}
	return nil
}

// ByID returns the mutual visible in the current practice, or insurer.ErrNotFound.
func (r *InsurerRepository) ByID(ctx context.Context, id insurer.ID) (insurer.Insurer, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return insurer.Insurer{}, err
	}

	row, err := queries.GetInsurerByID(ctx, toInsurerUUID(id))
	if err != nil {
		return insurer.Insurer{}, translateInsurerError(err)
	}
	return insurerToDomain(row)
}

// List returns the mutuals of the practice ordered by name.
func (r *InsurerRepository) List(ctx context.Context, activeOnly bool) ([]insurer.Insurer, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListInsurers(ctx, activeOnly)
	if err != nil {
		return nil, translateInsurerError(err)
	}

	subjects := make([]insurer.Insurer, 0, len(rows))
	for _, row := range rows {
		subject, err := insurerToDomain(row)
		if err != nil {
			return nil, err
		}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

// Update persists an edit of the name, the tax id or the active flag. A name taken by
// another mutual of the practice is insurer.ErrNameTaken, not a driver error.
func (r *InsurerRepository) Update(ctx context.Context, subject insurer.Insurer) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	_, err = queries.UpdateInsurer(ctx, sqlcgen.UpdateInsurerParams{
		ID:        toInsurerUUID(subject.ID),
		Name:      subject.Name,
		TaxID:     subject.TaxID,
		IsActive:  subject.IsActive,
		UpdatedAt: subject.UpdatedAt,
	})
	if err != nil {
		return translateInsurerError(err)
	}
	return nil
}

// insurerToDomain rebuilds the mutual from a row.
func insurerToDomain(row sqlcgen.Insurer) (insurer.Insurer, error) {
	subject, err := insurer.Reconstitute(insurer.Stored{
		ID:        insurer.ID(row.ID.String()),
		TenantID:  tenantIDOf(row.TenantID),
		Name:      row.Name,
		TaxID:     row.TaxID,
		IsActive:  row.IsActive,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	})
	if err != nil {
		return insurer.Insurer{}, fmt.Errorf("postgres: stored mutual %s is invalid: %w", row.ID.String(), err)
	}
	return subject, nil
}

// translateInsurerError maps storage failures onto the insurer domain's vocabulary.
func translateInsurerError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return insurer.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == insurersTenantNameUnique {
		return fmt.Errorf("%w: constraint %s", insurer.ErrNameTaken, pgErr.ConstraintName)
	}
	return err
}

// toInsurerUUID converts the domain identity into the pgx type. An identity that is not
// a UUID becomes the zero value, which matches no row: the caller sees not-found.
func toInsurerUUID(id insurer.ID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

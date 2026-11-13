package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// clinicalGrantsOneLive is the partial unique index that allows one live grant per
// patient and professional.
const clinicalGrantsOneLive = "clinical_grants_one_live"

// ClinicalGrantRepository is the PostgreSQL adapter for port.ClinicalGrantRepository.
//
// Like the other tenant tables it reads and writes only inside WithTenant: the grant
// rows are filtered by row-level security, never by a WHERE clause of this file.
type ClinicalGrantRepository struct {
	queries *sqlcgen.Queries
}

// NewClinicalGrantRepository builds the adapter over an existing pool.
func NewClinicalGrantRepository(pool *pgxpool.Pool) *ClinicalGrantRepository {
	return &ClinicalGrantRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.ClinicalGrantRepository = (*ClinicalGrantRepository)(nil)

// Create inserts a new grant. A second live grant for the same patient and
// professional is care.ErrGrantAlreadyLive.
func (r *ClinicalGrantRepository) Create(ctx context.Context, grant care.Grant) error {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return care.ErrTenantScopeMissing
	}

	tenantID, err := toPGUUIDFromTenant(grant.TenantID)
	if err != nil {
		return err
	}

	_, err = sqlcgen.New(tx).CreateClinicalGrant(ctx, sqlcgen.CreateClinicalGrantParams{
		ID:                 parseGrantUUID(grant.ID),
		TenantID:           tenantID,
		PatientID:          parseGrantUUID(grant.PatientID),
		ProfessionalUserID: parseGrantUUID(grant.ProfessionalUserID),
		GrantedBy:          parseGrantUUID(grant.GrantedBy),
		GrantedAt:          grant.GrantedAt,
		RevokedAt:          timestamptzOf(grant.RevokedAt),
	})
	if err != nil {
		return translateGrantError(err)
	}
	return nil
}

// ByID returns the grant, or care.ErrGrantNotFound.
func (r *ClinicalGrantRepository) ByID(ctx context.Context, id string) (care.Grant, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return care.Grant{}, care.ErrTenantScopeMissing
	}
	row, err := sqlcgen.New(tx).GetClinicalGrantByID(ctx, parseGrantUUID(id))
	if err != nil {
		return care.Grant{}, translateGrantError(err)
	}
	return grantToDomain(row), nil
}

// HasLiveGrant reports whether the professional holds an unrevoked grant on the patient.
func (r *ClinicalGrantRepository) HasLiveGrant(ctx context.Context, patientID, professionalUserID string) (bool, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return false, care.ErrTenantScopeMissing
	}
	live, err := sqlcgen.New(tx).HasLiveClinicalGrant(ctx, sqlcgen.HasLiveClinicalGrantParams{
		PatientID:          parseGrantUUID(patientID),
		ProfessionalUserID: parseGrantUUID(professionalUserID),
	})
	if err != nil {
		return false, translateGrantError(err)
	}
	return live, nil
}

// ListByPatient returns every grant over the patient, live and revoked, newest first.
func (r *ClinicalGrantRepository) ListByPatient(ctx context.Context, patientID string) ([]care.Grant, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, care.ErrTenantScopeMissing
	}
	rows, err := sqlcgen.New(tx).ListClinicalGrantsByPatient(ctx, parseGrantUUID(patientID))
	if err != nil {
		return nil, translateGrantError(err)
	}
	grants := make([]care.Grant, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, grantToDomain(row))
	}
	return grants, nil
}

// Save persists a revocation. Only RevokedAt changes after creation, so the statement
// writes that one column.
func (r *ClinicalGrantRepository) Save(ctx context.Context, grant care.Grant) error {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return care.ErrTenantScopeMissing
	}
	_, err := sqlcgen.New(tx).RevokeClinicalGrant(ctx, sqlcgen.RevokeClinicalGrantParams{
		ID:        parseGrantUUID(grant.ID),
		RevokedAt: timestamptzOf(grant.RevokedAt),
	})
	if err != nil {
		return translateGrantError(err)
	}
	return nil
}

// grantToDomain rebuilds the grant from a row.
func grantToDomain(row sqlcgen.ClinicalGrant) care.Grant {
	return care.Grant{
		ID:                 row.ID.String(),
		TenantID:           tenantIDOf(row.TenantID),
		PatientID:          row.PatientID.String(),
		ProfessionalUserID: row.ProfessionalUserID.String(),
		GrantedBy:          row.GrantedBy.String(),
		GrantedAt:          row.GrantedAt,
		RevokedAt:          timeOfTimestamptz(row.RevokedAt),
	}
}

// translateGrantError maps storage failures onto the care vocabulary.
func translateGrantError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return care.ErrGrantNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == clinicalGrantsOneLive:
			return fmt.Errorf("%w: constraint %s", care.ErrGrantAlreadyLive, pgErr.ConstraintName)
		case pgErr.Code == insufficientPrivilegeCode:
			return fmt.Errorf("%w: %s", care.ErrForbidden, pgErr.Message)
		}
	}
	return err
}

// parseGrantUUID turns an identity into the pgx type. A value that is not a UUID becomes
// the NULL UUID, which matches no row, so a lookup by it answers not-found and an insert
// with it fails loudly on the NOT NULL column instead of writing a wrong row.
func parseGrantUUID(value string) pgtype.UUID {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

// timestamptzOf maps an optional instant onto the pgx type.
func timestamptzOf(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

// timeOfTimestamptz maps a nullable instant back onto the domain.
func timeOfTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	when := value.Time
	return &when
}

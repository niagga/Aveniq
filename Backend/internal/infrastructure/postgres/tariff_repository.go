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
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// Constraint names behind the tariff errors the adapter translates.
const (
	tariffsValidityUnique         = "tariffs_validity_unique"
	tariffsProfessionalSameTenant = "tariffs_professional_same_tenant"
	tariffsInsurerSameTenant      = "tariffs_insurer_same_tenant"
)

// TariffRepository is the PostgreSQL adapter for port.TariffRepository.
//
// Like the insurer adapter it requires Transactor.WithTenant: row-level security derives
// the practice from app.tenant_id, so the queries carry no tenant filter.
type TariffRepository struct {
	queries *sqlcgen.Queries
}

// NewTariffRepository builds the adapter over an existing pool.
func NewTariffRepository(pool *pgxpool.Pool) *TariffRepository {
	return &TariffRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.TariffRepository = (*TariffRepository)(nil)

// scoped binds the queries to the open tenant transaction, failing instead of falling
// back to the pool.
func (r *TariffRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, pricing.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Create inserts a new tariff.
func (r *TariffRepository) Create(ctx context.Context, subject pricing.Tariff) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}
	professional, err := toTariffUUID(subject.ProfessionalUserID, "professional_user_id")
	if err != nil {
		return err
	}
	insurerID, err := optionalTariffUUID(subject.InsurerID, "insurer_id")
	if err != nil {
		return err
	}

	_, err = queries.CreateTariff(ctx, sqlcgen.CreateTariffParams{
		ID:                 toTariffID(subject.ID),
		TenantID:           tenantID,
		ProfessionalUserID: professional,
		Coverage:           string(subject.Coverage),
		InsurerID:          insurerID,
		Modality:           string(subject.Modality),
		BillingCurrency:    string(subject.Currency),
		ValidFrom:          pgtype.Date{Time: subject.ValidFrom, Valid: true},
		InsurerMinor:       subject.InsurerMinor,
		PatientMinor:       subject.PatientMinor,
		CreatedAt:          subject.CreatedAt,
		UpdatedAt:          subject.UpdatedAt,
	})
	if err != nil {
		return translateTariffError(err)
	}
	return nil
}

// ByID returns the tariff visible in the current practice, or pricing.ErrNotFound.
func (r *TariffRepository) ByID(ctx context.Context, id pricing.ID) (pricing.Tariff, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return pricing.Tariff{}, err
	}
	row, err := queries.GetTariffByID(ctx, toTariffID(id))
	if err != nil {
		return pricing.Tariff{}, translateTariffError(err)
	}
	return tariffToDomain(row)
}

// List returns every tariff of the practice with the names the screen shows.
func (r *TariffRepository) List(ctx context.Context) ([]port.TariffListItem, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListTariffs(ctx)
	if err != nil {
		return nil, translateTariffError(err)
	}

	items := make([]port.TariffListItem, 0, len(rows))
	for _, row := range rows {
		subject, err := tariffToDomain(sqlcgen.Tariff{
			ID:                 row.ID,
			TenantID:           row.TenantID,
			ProfessionalUserID: row.ProfessionalUserID,
			Coverage:           row.Coverage,
			InsurerID:          row.InsurerID,
			Modality:           row.Modality,
			BillingCurrency:    row.BillingCurrency,
			ValidFrom:          row.ValidFrom,
			InsurerMinor:       row.InsurerMinor,
			PatientMinor:       row.PatientMinor,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		})
		if err != nil {
			return nil, err
		}
		item := port.TariffListItem{Tariff: subject, ProfessionalName: row.ProfessionalName}
		if row.InsurerName != nil {
			item.InsurerName = *row.InsurerName
		}
		items = append(items, item)
	}
	return items, nil
}

// Update persists the amounts of a tariff. The key is not rewritten, so the statement
// touches only the split and the update time.
func (r *TariffRepository) Update(ctx context.Context, subject pricing.Tariff) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	_, err = queries.UpdateTariffAmounts(ctx, sqlcgen.UpdateTariffAmountsParams{
		ID:           toTariffID(subject.ID),
		InsurerMinor: subject.InsurerMinor,
		PatientMinor: subject.PatientMinor,
		UpdatedAt:    subject.UpdatedAt,
	})
	if err != nil {
		return translateTariffError(err)
	}
	return nil
}

// Delete removes a tariff. A tariff with no row in the practice is pricing.ErrNotFound.
func (r *TariffRepository) Delete(ctx context.Context, id pricing.ID) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	removed, err := queries.DeleteTariff(ctx, toTariffID(id))
	if err != nil {
		return translateTariffError(err)
	}
	if removed == 0 {
		return pricing.ErrNotFound
	}
	return nil
}

// tariffToDomain rebuilds the tariff from a row.
func tariffToDomain(row sqlcgen.Tariff) (pricing.Tariff, error) {
	subject, err := pricing.Reconstitute(pricing.Tariff{
		ID:                 pricing.ID(row.ID.String()),
		TenantID:           tenantIDOf(row.TenantID),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
		ProfessionalUserID: row.ProfessionalUserID.String(),
		Coverage:           patient.Coverage(row.Coverage),
		InsurerID:          optionalTariffUUIDString(row.InsurerID),
		Modality:           schedule.Modality(row.Modality),
		Currency:           patient.Currency(row.BillingCurrency),
		ValidFrom:          row.ValidFrom.Time,
		InsurerMinor:       row.InsurerMinor,
		PatientMinor:       row.PatientMinor,
	})
	if err != nil {
		return pricing.Tariff{}, fmt.Errorf("postgres: stored tariff %s is invalid: %w", row.ID.String(), err)
	}
	return subject, nil
}

// translateTariffError maps storage failures onto the pricing vocabulary.
func translateTariffError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == tariffsValidityUnique:
			return fmt.Errorf("%w: constraint %s", pricing.ErrValidityTaken, pgErr.ConstraintName)
		case pgErr.Code == foreignKeyViolationCode && pgErr.ConstraintName == tariffsProfessionalSameTenant:
			return &pricing.ValidationError{Field: "professional_user_id", Message: "must be a user of this practice"}
		case pgErr.Code == foreignKeyViolationCode && pgErr.ConstraintName == tariffsInsurerSameTenant:
			return &pricing.ValidationError{Field: "insurer_id", Message: "must be a mutual of this practice"}
		}
	}
	return err
}

// toTariffID converts the domain identity into the pgx type. An identity that is not a
// UUID becomes the zero value, which matches no row: the caller sees not-found.
func toTariffID(id pricing.ID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

// toTariffUUID converts a reference to another row. Unlike the identity, a reference that
// is not a UUID is a field error, because nothing in the practice can match it.
func toTariffUUID(value string, field string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, &pricing.ValidationError{Field: field, Message: "must be an identifier of this practice"}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

// optionalTariffUUID converts an optional reference; nil stays NULL.
func optionalTariffUUID(value *string, field string) (pgtype.UUID, error) {
	if value == nil {
		return pgtype.UUID{}, nil
	}
	return toTariffUUID(*value, field)
}

// optionalTariffUUIDString reads an optional reference back as text, nil when NULL.
func optionalTariffUUIDString(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := uuid.UUID(value.Bytes).String()
	return &text
}

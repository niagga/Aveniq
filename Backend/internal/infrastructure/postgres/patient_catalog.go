package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// PatientCatalog reads the country catalogs and resolves a practice's country.
//
// It deliberately does not need a tenant scope: the catalogs are global reference
// data and `tenants` has no row-level security. Reading them outside a transaction
// is what lets a use case normalize a document before opening the tenant scope it
// will write in.
type PatientCatalog struct {
	queries *sqlcgen.Queries
}

// NewPatientCatalog builds the adapter over an existing pool.
func NewPatientCatalog(pool *pgxpool.Pool) *PatientCatalog {
	return &PatientCatalog{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.DocumentCatalog = (*PatientCatalog)(nil)

// DocumentKind returns the rules for a personal document of a country.
func (c *PatientCatalog) DocumentKind(ctx context.Context, country, code string) (patient.DocumentKind, error) {
	row, err := c.queries.GetDocumentKind(ctx, sqlcgen.GetDocumentKindParams{
		CountryCode: country,
		Code:        code,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return patient.DocumentKind{}, fmt.Errorf(
				"%w: %s has no document kind %q", patient.ErrDocumentInvalid, country, code)
		}
		return patient.DocumentKind{}, translatePatientError(err)
	}

	return patient.DocumentKind{
		Country:    row.CountryCode,
		Code:       row.Code,
		Label:      row.Label,
		Pattern:    row.Pattern,
		Normalizer: row.Normalizer,
		Checksum:   derefString(row.Checksum),
	}, nil
}

// TaxIDKind returns the rules for a fiscal identifier of a country.
func (c *PatientCatalog) TaxIDKind(ctx context.Context, country, code string) (patient.DocumentKind, error) {
	row, err := c.queries.GetTaxIDKind(ctx, sqlcgen.GetTaxIDKindParams{
		CountryCode: country,
		Code:        code,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return patient.DocumentKind{}, fmt.Errorf(
				"%w: %s has no tax id kind %q", patient.ErrDocumentInvalid, country, code)
		}
		return patient.DocumentKind{}, translatePatientError(err)
	}

	return patient.DocumentKind{
		Country:    row.CountryCode,
		Code:       row.Code,
		Label:      row.Label,
		Pattern:    row.Pattern,
		Normalizer: row.Normalizer,
		Checksum:   derefString(row.Checksum),
	}, nil
}

// FiscalCondition confirms a country knows the condition before it is stored.
func (c *PatientCatalog) FiscalCondition(ctx context.Context, country, code string) (port.FiscalCondition, error) {
	row, err := c.queries.GetFiscalCondition(ctx, sqlcgen.GetFiscalConditionParams{
		CountryCode: country,
		Code:        code,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return port.FiscalCondition{}, fmt.Errorf(
				"%w: %s has no fiscal condition %q", patient.ErrDocumentInvalid, country, code)
		}
		return port.FiscalCondition{}, translatePatientError(err)
	}

	return port.FiscalCondition{Country: row.CountryCode, Code: row.Code, Label: row.Label}, nil
}

// TenantCountry is the country a patient inherits when the request does not choose
// one: the practice's.
func (c *PatientCatalog) TenantCountry(ctx context.Context, tenantID string) (string, error) {
	parsed, err := uuid.Parse(tenantID)
	if err != nil {
		return "", fmt.Errorf("postgres: %q is not a valid tenant UUID: %w", tenantID, err)
	}

	country, err := c.queries.GetTenantCountry(ctx, pgtype.UUID{Bytes: parsed, Valid: true})
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("%w: tenant %s", patient.ErrNotFound, tenantID)
		}
		return "", translatePatientError(err)
	}
	return country, nil
}

// derefString turns a nullable column into a plain string, where empty means absent.
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

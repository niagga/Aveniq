package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// ClientRepository is the PostgreSQL adapter for port.ClientRepository.
type ClientRepository struct {
	queries *sqlcgen.Queries
}

func NewClientRepository(pool *pgxpool.Pool) *ClientRepository {
	return &ClientRepository{queries: sqlcgen.New(pool)}
}

var _ port.ClientRepository = (*ClientRepository)(nil)

func (r *ClientRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, client.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

func (r *ClientRepository) Create(ctx context.Context, subject client.Client) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}
	_, err = queries.CreateClient(ctx, clientToCreateParams(subject, tenantID))
	if err != nil {
		return translateClientError(err)
	}
	return nil
}

func (r *ClientRepository) ByID(ctx context.Context, id client.ID) (client.Client, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return client.Client{}, err
	}
	row, err := queries.GetClientByID(ctx, toClientUUID(id))
	if err != nil {
		return client.Client{}, translateClientError(err)
	}
	return clientToDomain(row)
}

func (r *ClientRepository) List(ctx context.Context, activeOnly bool) ([]client.Client, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListClients(ctx, activeOnly)
	if err != nil {
		return nil, translateClientError(err)
	}
	subjects := make([]client.Client, 0, len(rows))
	for _, row := range rows {
		subject, err := clientToDomain(row)
		if err != nil {
			return nil, err
		}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

func (r *ClientRepository) Update(ctx context.Context, subject client.Client) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}
	_, err = queries.UpdateClient(ctx, clientToUpdateParams(subject, tenantID))
	if err != nil {
		return translateClientError(err)
	}
	return nil
}

func clientToCreateParams(subject client.Client, tenantID pgtype.UUID) sqlcgen.CreateClientParams {
	taxNumber := taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Number })
	taxKind := taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Kind })
	return sqlcgen.CreateClientParams{
		ID:              toClientUUID(subject.ID),
		TenantID:        tenantID,
		DisplayName:     subject.DisplayName,
		LegalName:       subject.LegalName,
		Country:         subject.Country,
		FiscalCondition: subject.FiscalCondition,
		TaxID:           taxNumber,
		TaxIDKind:       taxKind,
		FiscalAddress:   subject.FiscalAddress,
		BillingCurrency: string(subject.BillingCurrency),
		IsActive:        subject.IsActive,
		CreatedAt:       subject.CreatedAt,
		UpdatedAt:       subject.UpdatedAt,
	}
}

func clientToUpdateParams(subject client.Client, tenantID pgtype.UUID) sqlcgen.UpdateClientParams {
	taxNumber := taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Number })
	taxKind := taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Kind })
	return sqlcgen.UpdateClientParams{
		ID:              toClientUUID(subject.ID),
		DisplayName:     subject.DisplayName,
		LegalName:       subject.LegalName,
		Country:         subject.Country,
		FiscalCondition: subject.FiscalCondition,
		TaxID:           taxNumber,
		TaxIDKind:       taxKind,
		FiscalAddress:   subject.FiscalAddress,
		BillingCurrency: string(subject.BillingCurrency),
		IsActive:        subject.IsActive,
		UpdatedAt:       subject.UpdatedAt,
	}
}

func clientToDomain(row sqlcgen.Client) (client.Client, error) {
	var taxID *patient.TaxID
	if row.TaxID != nil && row.TaxIDKind != nil {
		taxID = &patient.TaxID{
			Kind:       *row.TaxIDKind,
			Number:     *row.TaxID,
			Normalized: *row.TaxID,
		}
	}
	subject, err := client.Reconstitute(client.Stored{
		ID:              client.ID(row.ID.String()),
		TenantID:        tenantIDOf(row.TenantID),
		DisplayName:     row.DisplayName,
		LegalName:       row.LegalName,
		Country:         row.Country,
		TaxID:           taxID,
		FiscalCondition: row.FiscalCondition,
		FiscalAddress:   row.FiscalAddress,
		BillingCurrency: patient.Currency(row.BillingCurrency),
		IsActive:        row.IsActive,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	})
	if err != nil {
		return client.Client{}, fmt.Errorf("postgres: stored client %s is invalid: %w", row.ID.String(), err)
	}
	return subject, nil
}

func translateClientError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return client.ErrNotFound
	}
	return err
}

func toClientUUID(id client.ID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}
package postgres_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

const clientInsertOwnerSQL = `
	INSERT INTO clients (id, tenant_id, display_name, country, billing_currency, is_active, created_at, updated_at)
	VALUES ($1, $2, $3, $4, 'ARS', true, now(), now())`

const clientInsertAppSQL = `
	INSERT INTO clients (id, tenant_id, display_name, country, billing_currency, is_active, created_at, updated_at)
	VALUES ($1, $2, $3, $4, 'ARS', true, now(), now())`

// schemaPatientsHaveClientID is true after migration 0026.
func schemaPatientsHaveClientID(ctx context.Context, conn *pgx.Conn) bool {
	var ok bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'patients'
			  AND column_name = 'client_id')`,
	).Scan(&ok); err != nil {
		return false
	}
	return ok
}

// insertClientOwner seeds a billing client as the migrator. Used in fixtures that
// already connect as the owner.
func insertClientOwner(ctx context.Context, conn *pgx.Conn, tenantID, displayName, country string) (string, error) {
	if country == "" {
		country = "AR"
	}
	id := uuid.NewString()
	_, err := conn.Exec(ctx, clientInsertOwnerSQL, id, tenantID, displayName, country)
	return id, err
}

// insertClientApp seeds a billing client under the open tenant scope.
func insertClientApp(ctx context.Context, conn *pgx.Conn, tenantID, displayName, country string) (string, error) {
	if country == "" {
		country = "AR"
	}
	id := uuid.NewString()
	_, err := conn.Exec(ctx, clientInsertAppSQL, id, tenantID, displayName, country)
	return id, err
}

// persistPatient creates the default billing client and the patient in one tenant scope,
// matching what the application does after migration 0026.
func persistPatient(
	ctx context.Context,
	clients *postgres.ClientRepository,
	repo *postgres.PatientRepository,
	subject patient.Patient,
) error {
	payer, err := client.New(
		client.ID(uuid.NewString()),
		subject.TenantID,
		clientDraftFromPatient(subject),
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if err := clients.Create(ctx, payer); err != nil {
		return err
	}
	subject.ClientID = string(payer.ID)
	return repo.Create(ctx, subject)
}

func clientDraftFromPatient(subject patient.Patient) client.Draft {
	return client.Draft{
		DisplayName:     subject.FullName,
		Country:         subject.Country,
		TaxID:           subject.TaxID,
		FiscalCondition: subject.FiscalCondition,
		FiscalAddress:   subject.FiscalAddress,
		BillingCurrency: subject.BillingCurrency,
	}
}
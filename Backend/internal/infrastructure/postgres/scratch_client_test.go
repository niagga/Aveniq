package postgres_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
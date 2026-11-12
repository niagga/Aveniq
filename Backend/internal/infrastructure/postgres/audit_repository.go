package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// AuditRepository is the PostgreSQL adapter for port.AuditLog.
//
// It writes through the tenant transaction carried in the context, not through the
// pool. That is not an optimisation: if the audit insert used its own connection, a
// clinical read could commit and its trail could fail independently, and then the
// product would hold clinical data with no record of who looked at it.
//
// Since the row is written inside the caller's transaction, a failure to audit fails
// the read. Fail closed is the chosen behaviour (L26): the alternative is a read that
// silently has no trail, which is the exact thing the table exists to prevent.
type AuditRepository struct {
	queries *sqlcgen.Queries
}

// NewAuditRepository builds the adapter over an existing pool.
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.AuditLog = (*AuditRepository)(nil)

// Record appends one entry.
func (r *AuditRepository) Record(ctx context.Context, event port.AuditEvent) error {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return clinical.ErrTenantScopeMissing
	}

	tenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		return fmt.Errorf("postgres: audit event with an invalid tenant %q: %w", event.TenantID, err)
	}
	actorID, err := uuid.Parse(event.ActorID)
	if err != nil {
		return fmt.Errorf("postgres: audit event with an invalid actor %q: %w", event.ActorID, err)
	}

	metadata := event.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("postgres: cannot encode the audit metadata: %w", err)
	}

	var entityID *string
	if event.EntityID != "" {
		value := event.EntityID
		entityID = &value
	}

	if err := r.queries.WithTx(tx).CreateAuditEntry(ctx, sqlcgen.CreateAuditEntryParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		ActorID:  pgtype.UUID{Bytes: actorID, Valid: true},
		Action:   event.Action,
		Entity:   event.Entity,
		EntityID: entityID,
		Metadata: encoded,
	}); err != nil {
		return fmt.Errorf("postgres: cannot record the audit entry: %w", translateClinicalError(err))
	}
	return nil
}

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// tenantContextKey carries the open tenant transaction through the context.
// It is unexported on purpose: only WithTenant may put one in, and only the
// repositories in this package may take one out.
type tenantContextKey struct{}

// Transactor scopes units of work to one tenant per transaction.
//
// Decision 4 of L6: the tenant context is set PER TRANSACTION with the
// transactional form of SET LOCAL, never per connection with plain SET.
// Connections come from a pool and are reused by requests from different
// tenants, so a connection-level setting would leak one practice's tenant into
// another practice's request.
//
// Repositories must be called inside fn: their statements run against the open
// transaction, where row-level security can see app.tenant_id. Outside it, the
// policies fail closed — reads return zero rows and writes raise SQLSTATE 42501.
type Transactor struct {
	pool *pgxpool.Pool
}

// NewTransactor builds the transactor over an existing pool.
func NewTransactor(pool *pgxpool.Pool) *Transactor {
	return &Transactor{pool: pool}
}

// WithTenant opens a transaction, sets app.tenant_id for it, and runs fn with a
// context that carries the transaction. A nil error commits; any other error
// rolls back and is returned unchanged.
//
// set_config('app.tenant_id', $1, true) is the parameterized form of
// `SET LOCAL app.tenant_id = $1`: SET LOCAL cannot take a bind parameter, and
// tenant identifiers must never be interpolated into SQL text.
func (t *Transactor) WithTenant(ctx context.Context, tenantID string, fn func(ctx context.Context) error) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return fmt.Errorf("postgres: %q is not a valid tenant UUID: %w", tenantID, err)
	}

	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: cannot begin the tenant transaction: %w", err)
	}

	// The deferred rollback is a safety net, not the main path: the transaction
	// holds a pooled connection until it is closed, so a panic inside fn would
	// otherwise leak that connection for the life of the process. After a
	// successful Commit it is a no-op returning pgx.ErrTxClosed, which is why
	// the explicit rollbacks below are unnecessary.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("postgres: cannot set the tenant context: %w", err)
	}

	if err := fn(context.WithValue(ctx, tenantContextKey{}, tx)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// tenantTransaction returns the transaction WithTenant opened on this context.
func tenantTransaction(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(tenantContextKey{}).(pgx.Tx)
	return tx, ok
}

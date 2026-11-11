package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// SessionRepository is the PostgreSQL adapter for port.SessionRepository.
//
// Like the user adapter, every method requires Transactor.WithTenant: the tenant
// comes from app.tenant_id and the queries carry no tenant filter. Called outside
// a tenant transaction the adapter returns iam.ErrTenantScopeMissing before
// reaching the database.
type SessionRepository struct {
	queries *sqlcgen.Queries
}

// NewSessionRepository builds the adapter over an existing pool.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port, asserted against the
// port itself so a rename there breaks the build here.
var _ port.SessionRepository = (*SessionRepository)(nil)

// scoped returns the query layer bound to the open tenant transaction, failing
// instead of falling back to the pool.
func (r *SessionRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, iam.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Create inserts a new session, after proving the user is visible in the current
// tenant scope.
//
// The visibility check is decision 7 of L11. `sessions.user_id` has a foreign key
// to `users.id`, but a foreign key check does not run through row-level security:
// knowing a user UUID from another tenant would be enough to attach a session to
// it, and the login path would then authenticate a cross-tenant identity. Asking
// the scoped query first makes "invisible" and "does not exist" the same answer,
// which is also the answer that leaks nothing.
func (r *SessionRepository) Create(ctx context.Context, subject iam.Session) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	if _, err := queries.SessionUserVisible(ctx, toPGUUIDFromIAM(subject.UserID)); err != nil {
		if translated := translateIAMError(err); translated != err {
			return fmt.Errorf("%w: the user is not visible in this tenant scope", translated)
		}
		return err
	}

	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}

	_, err = queries.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID:          toPGUUIDFromIAM(iam.UserID(subject.ID)),
		TenantID:    tenantID,
		UserID:      toPGUUIDFromIAM(subject.UserID),
		RefreshHash: string(subject.RefreshHash),
		UserAgent:   subject.UserAgent,
		Ip:          subject.IP,
		CreatedAt:   subject.CreatedAt,
		ExpiresAt:   subject.ExpiresAt,
	})
	if err != nil {
		return translateIAMError(err)
	}
	return nil
}

// ByRefreshHash returns the session that holds the given hash, revoked or not.
//
// Returning revoked rows is deliberate: reuse detection needs to find the row
// whose `revoked_at` is already set and kill the chain. Filtering them out here
// would turn "this token was already used" into "this token never existed".
func (r *SessionRepository) ByRefreshHash(ctx context.Context, hash iam.RefreshHash) (iam.Session, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return iam.Session{}, err
	}

	row, err := queries.GetSessionByRefreshHash(ctx, string(hash))
	if err != nil {
		return iam.Session{}, translateIAMError(err)
	}
	return sessionToDomain(row)
}

// Rotate persists a rotation: the successor is inserted first, then the presented
// grant is marked revoked and pointed at it.
//
// The order is forced by the schema: `replaced_by` is a foreign key to `sessions`,
// so the successor must exist before the reference is written. Both statements run
// in the caller's transaction, so a failure leaves neither.
//
// A zero-row update means the presented grant was already revoked. That is
// iam.ErrInvalidTransition and not iam.ErrNotFound on purpose: the caller must be
// able to tell "this token was already spent, someone may be replaying it" from
// "this token was never issued".
func (r *SessionRepository) Rotate(ctx context.Context, previous, successor iam.Session) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	if previous.RevokedAt == nil {
		return fmt.Errorf("%w: the previous session must carry its revocation", iam.ErrInvalidTransition)
	}

	tenantID, err := toPGUUIDFromTenant(successor.TenantID)
	if err != nil {
		return err
	}
	if _, err := queries.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID:          toPGUUIDFromIAM(iam.UserID(successor.ID)),
		TenantID:    tenantID,
		UserID:      toPGUUIDFromIAM(successor.UserID),
		RefreshHash: string(successor.RefreshHash),
		UserAgent:   successor.UserAgent,
		Ip:          successor.IP,
		CreatedAt:   successor.CreatedAt,
		ExpiresAt:   successor.ExpiresAt,
	}); err != nil {
		return translateIAMError(err)
	}

	affected, err := queries.RotateSession(ctx, sqlcgen.RotateSessionParams{
		ID:         toPGUUIDFromIAM(iam.UserID(previous.ID)),
		RevokedAt:  toPGTimestamptz(previous.RevokedAt),
		ReplacedBy: toPGUUIDFromIAM(iam.UserID(successor.ID)),
	})
	if err != nil {
		return translateIAMError(err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: the session was already revoked", iam.ErrInvalidTransition)
	}
	return nil
}

// Revoke marks one session dead. A missing (or already revoked) session is
// iam.ErrNotFound so a caller cannot distinguish it from an invisible one.
func (r *SessionRepository) Revoke(ctx context.Context, id iam.SessionID, at time.Time) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	affected, err := queries.RevokeSession(ctx, sqlcgen.RevokeSessionParams{
		ID:        toPGUUIDFromIAM(iam.UserID(id)),
		RevokedAt: toPGTimestamptz(&at),
	})
	if err != nil {
		return translateIAMError(err)
	}
	if affected == 0 {
		return iam.ErrNotFound
	}
	return nil
}

// RevokeChain revokes the presented session and every successor reachable through
// `replaced_by`, returning how many rows it changed.
//
// Decision 3 of L11: a replayed token kills the chain, not the leaf. The stolen
// copy and the legitimate client both hold a token of the same chain, and nothing
// in the request says which is which, so the only safe move is to end the whole
// line and force a fresh login.
func (r *SessionRepository) RevokeChain(ctx context.Context, id iam.SessionID, at time.Time) (int64, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return 0, err
	}

	affected, err := queries.RevokeSessionChain(ctx, sqlcgen.RevokeSessionChainParams{
		ID:        toPGUUIDFromIAM(iam.UserID(id)),
		RevokedAt: toPGTimestamptz(&at),
	})
	if err != nil {
		return 0, translateIAMError(err)
	}
	if affected == 0 {
		return 0, iam.ErrNotFound
	}
	return affected, nil
}

// sessionToDomain rebuilds the aggregate through its validating constructor.
func sessionToDomain(row sqlcgen.Session) (iam.Session, error) {
	session, err := iam.ReconstituteSession(
		iam.SessionID(row.ID.String()),
		tenantIDOf(row.TenantID),
		iam.UserID(row.UserID.String()),
		iam.RefreshHash(row.RefreshHash),
		row.UserAgent,
		row.Ip,
		row.CreatedAt,
		row.ExpiresAt,
		timestamptzFromPG(row.RevokedAt),
		sessionIDFromPG(row.ReplacedBy),
	)
	if err != nil {
		return iam.Session{}, fmt.Errorf("postgres: stored session %s is invalid: %w", row.ID.String(), err)
	}
	return session, nil
}

// sessionIDFromPG maps the nullable successor reference back onto the domain.
func sessionIDFromPG(value pgtype.UUID) *iam.SessionID {
	if !value.Valid {
		return nil
	}
	id := iam.SessionID(value.String())
	return &id
}

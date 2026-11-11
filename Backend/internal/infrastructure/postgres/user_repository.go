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

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// rlsViolationCode is PostgreSQL's SQLSTATE for "insufficient privilege", which
// is what a WITH CHECK policy raises when app.tenant_id was never set.
const rlsViolationCode = "42501"

// usersTenantEmailUnique is the constraint behind the per-tenant email
// uniqueness: the same human may belong to two practices, the same address
// cannot appear twice in one.
const usersTenantEmailUnique = "users_tenant_email_unique"

// UserRepository is the PostgreSQL adapter for port.UserRepository.
//
// Every method assumes it runs inside Transactor.WithTenant: row-level security
// derives the tenant from app.tenant_id, so the queries carry no tenant filter
// of their own. Called outside a tenant transaction, the policies fail closed —
// reads return zero rows (mapped to iam.ErrNotFound) and writes raise 42501
// (mapped to iam.ErrTenantScopeMissing).
type UserRepository struct {
	queries *sqlcgen.Queries
}

// NewUserRepository builds the adapter over an existing pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ interface {
	Create(ctx context.Context, subject iam.User) error
	ByID(ctx context.Context, id iam.UserID) (iam.User, error)
	ByEmail(ctx context.Context, email string) (iam.User, error)
	List(ctx context.Context, limit, offset int32) ([]iam.User, error)
	Update(ctx context.Context, subject iam.User) error
	Delete(ctx context.Context, id iam.UserID) error
} = (*UserRepository)(nil)

// scoped returns the query layer bound to the open tenant transaction when one
// is in the context, or the pool otherwise. The pool path is what makes an
// out-of-scope mutation impossible rather than dangerous: RLS rejects it.
func (r *UserRepository) scoped(ctx context.Context) *sqlcgen.Queries {
	if tx, ok := tenantTransaction(ctx); ok {
		return r.queries.WithTx(tx)
	}
	return r.queries
}

// Create inserts a new user, translating uniqueness conflicts and a missing
// tenant scope into domain errors so the application layer never has to
// recognise a driver-specific code.
func (r *UserRepository) Create(ctx context.Context, subject iam.User) error {
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}

	_, err = r.scoped(ctx).CreateUser(ctx, sqlcgen.CreateUserParams{
		ID:              toPGUUIDFromIAM(subject.ID),
		TenantID:        tenantID,
		Email:           subject.Email,
		PasswordHash:    string(subject.PasswordHash),
		FullName:        subject.FullName,
		RoleCode:        string(subject.Role),
		IsActive:        subject.IsActive,
		EmailVerifiedAt: toPGTimestamptz(subject.EmailVerifiedAt),
		MfaSecret:       subject.MFASecret,
		CreatedAt:       subject.CreatedAt,
		UpdatedAt:       subject.UpdatedAt,
	})
	if err != nil {
		return translateIAMError(err)
	}
	return nil
}

// ByID returns the user with the given identity, visible only if it belongs to
// the current tenant, or iam.ErrNotFound.
func (r *UserRepository) ByID(ctx context.Context, id iam.UserID) (iam.User, error) {
	row, err := r.scoped(ctx).GetUserByID(ctx, toPGUUIDFromIAM(id))
	if err != nil {
		return iam.User{}, translateIAMError(err)
	}
	return userToDomain(row)
}

// ByEmail resolves a user by canonical email within the current tenant.
func (r *UserRepository) ByEmail(ctx context.Context, email string) (iam.User, error) {
	row, err := r.scoped(ctx).GetUserByEmail(ctx, email)
	if err != nil {
		return iam.User{}, translateIAMError(err)
	}
	return userToDomain(row)
}

// List returns one page of the current tenant's users, newest first.
func (r *UserRepository) List(ctx context.Context, limit, offset int32) ([]iam.User, error) {
	rows, err := r.scoped(ctx).ListUsers(ctx, sqlcgen.ListUsersParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, translateIAMError(err)
	}

	users := make([]iam.User, 0, len(rows))
	for _, row := range rows {
		user, err := userToDomain(row)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

// Update persists the mutable state of the aggregate. The email is deliberately
// absent: it is the login identity and changing it is a future decision, not a
// side effect of UpdateUser.
func (r *UserRepository) Update(ctx context.Context, subject iam.User) error {
	_, err := r.scoped(ctx).UpdateUser(ctx, sqlcgen.UpdateUserParams{
		ID:              toPGUUIDFromIAM(subject.ID),
		PasswordHash:    string(subject.PasswordHash),
		FullName:        subject.FullName,
		RoleCode:        string(subject.Role),
		IsActive:        subject.IsActive,
		EmailVerifiedAt: toPGTimestamptz(subject.EmailVerifiedAt),
		MfaSecret:       subject.MFASecret,
		UpdatedAt:       subject.UpdatedAt,
	})
	if err != nil {
		return translateIAMError(err)
	}
	return nil
}

// Delete removes the user. A missing (or invisible) user is iam.ErrNotFound.
func (r *UserRepository) Delete(ctx context.Context, id iam.UserID) error {
	affected, err := r.scoped(ctx).DeleteUser(ctx, toPGUUIDFromIAM(id))
	if err != nil {
		return translateIAMError(err)
	}
	if affected == 0 {
		return iam.ErrNotFound
	}
	return nil
}

// userToDomain rebuilds the aggregate through its validating constructor.
func userToDomain(row sqlcgen.User) (iam.User, error) {
	user, err := iam.ReconstituteUser(
		iam.UserID(row.ID.String()),
		tenantIDOf(row.TenantID),
		row.Email,
		iam.PasswordHash(row.PasswordHash),
		row.FullName,
		iam.RoleCode(row.RoleCode),
		row.IsActive,
		timestamptzFromPG(row.EmailVerifiedAt),
		row.MfaSecret,
		row.CreatedAt,
		row.UpdatedAt,
	)
	if err != nil {
		return iam.User{}, fmt.Errorf("postgres: stored user %s is invalid: %w", row.ID.String(), err)
	}
	return user, nil
}

// translateIAMError maps storage failures onto the IAM domain's vocabulary.
func translateIAMError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return iam.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == usersTenantEmailUnique:
			return fmt.Errorf("%w: constraint %s", iam.ErrEmailTaken, pgErr.ConstraintName)
		case pgErr.Code == rlsViolationCode:
			// The only way this adapter loses a privilege is the row-level
			// security policy: no tenant context, no access. Fail loudly in
			// the domain's vocabulary instead of leaking SQLSTATE.
			return iam.ErrTenantScopeMissing
		}
	}

	return err
}

// toPGUUIDFromIAM converts a domain user ID into the pgx type. An invalid UUID
// in a domain ID is a programmer error: the domain constructor accepts any
// non-empty string, but the schema cannot store it. Callers get a zero UUID and
// Postgres rejects the statement, surfacing as a driver error instead of a
// corrupted row.
func toPGUUIDFromIAM(id iam.UserID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

// toPGUUIDFromTenant converts a domain tenant identity into the pgx type.
func toPGUUIDFromTenant(id tenant.ID) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("postgres: %q is not a valid tenant UUID: %w", id, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

// tenantIDOf converts a stored tenant UUID back into the domain identity.
func tenantIDOf(value pgtype.UUID) tenant.ID {
	return tenant.ID(value.String())
}

// toPGTimestamptz maps the domain's optional timestamps onto the pgx type.
func toPGTimestamptz(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

// timestamptzFromPG maps a stored nullable timestamp back onto the domain.
func timestamptzFromPG(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

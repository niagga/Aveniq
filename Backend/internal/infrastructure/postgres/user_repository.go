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
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// insufficientPrivilegeCode is PostgreSQL's SQLSTATE for "insufficient
// privilege". It covers a missing grant as well as a WITH CHECK policy refusing
// an insert, which is exactly why it must not be reported as a scope problem.
const insufficientPrivilegeCode = "42501"

// usersTenantEmailUnique is the constraint behind the per-tenant email
// uniqueness: the same human may belong to two practices, the same address
// cannot appear twice in one.
const usersTenantEmailUnique = "users_tenant_email_unique"

// UserRepository is the PostgreSQL adapter for port.UserRepository.
//
// Every method requires Transactor.WithTenant: row-level security derives the
// tenant from app.tenant_id, so the queries carry no tenant filter of their own.
// Called outside a tenant transaction the adapter returns
// iam.ErrTenantScopeMissing before touching the database, so a missing scope is
// always loud and never disguised as an empty result.
type UserRepository struct {
	queries *sqlcgen.Queries
}

// NewUserRepository builds the adapter over an existing pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port. Asserting the port
// itself, and not a copy of its method set, is what turns a rename in port.go
// into a build failure here instead of a surprise at the composition root.
var _ port.UserRepository = (*UserRepository)(nil)

// scoped returns the query layer bound to the open tenant transaction. It fails
// instead of falling back to the pool: a pooled connection carries no tenant
// setting, so a statement outside a transaction would be filtered by the
// policies into zero rows and look like an ordinary not-found. An explicit error
// says what actually happened.
func (r *UserRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, iam.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Create inserts a new user, translating uniqueness conflicts and a missing
// tenant scope into domain errors so the application layer never has to
// recognise a driver-specific code.
func (r *UserRepository) Create(ctx context.Context, subject iam.User) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}

	_, err = queries.CreateUser(ctx, sqlcgen.CreateUserParams{
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
	queries, err := r.scoped(ctx)
	if err != nil {
		return iam.User{}, err
	}

	row, err := queries.GetUserByID(ctx, toPGUUIDFromIAM(id))
	if err != nil {
		return iam.User{}, translateIAMError(err)
	}
	return userToDomain(row)
}

// ByEmail resolves a user by canonical email within the current tenant.
func (r *UserRepository) ByEmail(ctx context.Context, email string) (iam.User, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return iam.User{}, err
	}

	row, err := queries.GetUserByEmail(ctx, email)
	if err != nil {
		return iam.User{}, translateIAMError(err)
	}
	return userToDomain(row)
}

// List returns one page of the current tenant's users, newest first.
func (r *UserRepository) List(ctx context.Context, limit, offset int32) ([]iam.User, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListUsers(ctx, sqlcgen.ListUsersParams{
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
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	_, err = queries.UpdateUser(ctx, sqlcgen.UpdateUserParams{
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
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	affected, err := queries.DeleteUser(ctx, toPGUUIDFromIAM(id))
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
		case pgErr.Code == insufficientPrivilegeCode:
			// A missing grant and a WITH CHECK rejection share this SQLSTATE.
			// Neither is a scope problem — the adapter already refuses to run
			// without a tenant transaction — so report the privilege honestly
			// instead of sending whoever reads the logs after the wrong bug.
			return fmt.Errorf("%w: %s", iam.ErrInsufficientPrivilege, pgErr.Message)
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

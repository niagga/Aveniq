package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// InvitationRepository is the PostgreSQL adapter for port.InvitationRepository.
//
// Like the user adapter, every method requires Transactor.WithTenant. The
// single-use guard lives in the UPDATE and the DELETE statements, so two requests
// racing on one link cannot both win.
type InvitationRepository struct {
	queries *sqlcgen.Queries
}

// NewInvitationRepository builds the adapter over an existing pool.
func NewInvitationRepository(pool *pgxpool.Pool) *InvitationRepository {
	return &InvitationRepository{queries: sqlcgen.New(pool)}
}

var _ port.InvitationRepository = (*InvitationRepository)(nil)

func (r *InvitationRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, iam.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Create stores a pending invitation.
func (r *InvitationRepository) Create(ctx context.Context, subject iam.Invitation) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	id, err := invitationUUID(string(subject.ID))
	if err != nil {
		return err
	}
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return err
	}
	_, err = queries.CreateInvitation(ctx, sqlcgen.CreateInvitationParams{
		ID:        id,
		TenantID:  tenantID,
		Email:     subject.Email,
		RoleCode:  string(subject.Role),
		TokenHash: subject.TokenHash,
		InvitedBy: toPGUUIDFromIAM(subject.InvitedBy),
		ExpiresAt: subject.ExpiresAt,
		CreatedAt: subject.CreatedAt,
	})
	if err != nil {
		return translateIAMError(err)
	}
	return nil
}

// ByID returns an invitation of the current practice or iam.ErrNotFound.
func (r *InvitationRepository) ByID(ctx context.Context, id iam.InvitationID) (iam.Invitation, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return iam.Invitation{}, err
	}

	parsed, err := invitationUUID(string(id))
	if err != nil {
		return iam.Invitation{}, iam.ErrNotFound
	}
	row, err := queries.GetInvitationByID(ctx, parsed)
	if err != nil {
		return iam.Invitation{}, translateIAMError(err)
	}
	return invitationToDomain(row)
}

// ByTokenHash resolves the invitation a link secret belongs to, within the scope
// the token's tenant hint opened.
func (r *InvitationRepository) ByTokenHash(ctx context.Context, hash string) (iam.Invitation, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return iam.Invitation{}, err
	}

	row, err := queries.GetInvitationByTokenHash(ctx, hash)
	if err != nil {
		return iam.Invitation{}, translateIAMError(err)
	}
	return invitationToDomain(row)
}

// Pending lists the invitations a practice can still act on.
func (r *InvitationRepository) Pending(ctx context.Context, now time.Time, limit, offset int32) ([]iam.Invitation, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListPendingInvitations(ctx, sqlcgen.ListPendingInvitationsParams{
		ExpiresAt: now.UTC(),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, translateIAMError(err)
	}

	invitations := make([]iam.Invitation, 0, len(rows))
	for _, row := range rows {
		invitation, err := invitationToDomain(row)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, invitation)
	}
	return invitations, nil
}

// MarkAccepted flips the invitation to accepted, but only while it is still usable.
// Zero affected rows means it was accepted or expired by the time the UPDATE ran.
func (r *InvitationRepository) MarkAccepted(ctx context.Context, id iam.InvitationID, at time.Time) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	parsed, err := invitationUUID(string(id))
	if err != nil {
		return iam.ErrInvitationUnusable
	}
	affected, err := queries.MarkInvitationAccepted(ctx, sqlcgen.MarkInvitationAcceptedParams{
		ID:         parsed,
		AcceptedAt: toPGTimestamptz(&at),
	})
	if err != nil {
		return translateIAMError(err)
	}
	if affected == 0 {
		return iam.ErrInvitationUnusable
	}
	return nil
}

// Delete revokes a pending invitation. Zero rows means it was accepted in the
// meantime, or never existed in this practice; the caller has already checked the
// first case, so a zero here is reported as not found.
func (r *InvitationRepository) Delete(ctx context.Context, id iam.InvitationID) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	parsed, err := invitationUUID(string(id))
	if err != nil {
		return iam.ErrNotFound
	}
	affected, err := queries.DeleteInvitation(ctx, parsed)
	if err != nil {
		return translateIAMError(err)
	}
	if affected == 0 {
		return iam.ErrNotFound
	}
	return nil
}

// invitationToDomain rebuilds the aggregate through its validating constructor.
func invitationToDomain(row sqlcgen.Invitation) (iam.Invitation, error) {
	var invitedBy iam.UserID
	if row.InvitedBy.Valid {
		invitedBy = iam.UserID(uuid.UUID(row.InvitedBy.Bytes).String())
	}

	invitation, err := iam.ReconstituteInvitation(
		iam.InvitationID(uuid.UUID(row.ID.Bytes).String()),
		tenantIDOf(row.TenantID),
		row.Email,
		iam.RoleCode(row.RoleCode),
		row.TokenHash,
		invitedBy,
		row.ExpiresAt,
		timestamptzFromPG(row.AcceptedAt),
		row.CreatedAt,
	)
	if err != nil {
		return iam.Invitation{}, fmt.Errorf("postgres: stored invitation %s is invalid: %w", uuid.UUID(row.ID.Bytes).String(), err)
	}
	return invitation, nil
}

// invitationUUID parses an identifier from the domain. An unparseable one cannot
// exist in the table, so callers turn it into not-found rather than a driver error.
func invitationUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("postgres: %q is not a valid UUID: %w", value, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

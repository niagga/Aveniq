package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// ClinicalRepository is the PostgreSQL adapter for port.ClinicalRepository.
//
// It belongs to the group of adapters that cannot run outside a tenant transaction:
// row-level security derives the tenant from app.tenant_id, so a statement on the pool
// would be filtered into zero rows and look like an ordinary not-found. scoped()
// refuses before that happens.
//
// The strongest guarantee here is not in this file. `clinical_notes` and
// `clinical_records` carry the tenant inside their foreign keys (0007), so a row
// pointing at another practice's patient is refused by the database even if this
// adapter were wrong.
type ClinicalRepository struct {
	queries *sqlcgen.Queries
}

// NewClinicalRepository builds the adapter over an existing pool.
func NewClinicalRepository(pool *pgxpool.Pool) *ClinicalRepository {
	return &ClinicalRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.ClinicalRepository = (*ClinicalRepository)(nil)

// scoped returns the query layer bound to the open tenant transaction.
func (r *ClinicalRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, clinical.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// UpsertRecord writes the frame of a treatment, creating it the first time.
//
// The upsert is what makes the endpoint idempotent from the client's point of view:
// the screen saves a record it may not have created yet, and "one record per patient"
// is enforced by the unique key rather than by the caller remembering to check.
func (r *ClinicalRepository) UpsertRecord(ctx context.Context, record clinical.Record) (clinical.Record, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return clinical.Record{}, err
	}

	tenantID, err := toPGUUIDFromTenant(record.TenantID)
	if err != nil {
		return clinical.Record{}, err
	}
	patientID, err := parseUUID(record.PatientID, "patient_id")
	if err != nil {
		return clinical.Record{}, err
	}

	row, err := queries.UpsertClinicalRecord(ctx, sqlcgen.UpsertClinicalRecordParams{
		ID:             toRecordUUID(record.ID),
		TenantID:       tenantID,
		PatientID:      patientID,
		ChiefComplaint: record.ChiefComplaint,
		Background:     record.Background,
		Objectives:     record.Objectives,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	})
	if err != nil {
		return clinical.Record{}, translateClinicalError(err)
	}
	return recordToDomain(row)
}

// RecordByPatient returns the frame of a patient, or clinical.ErrNotFound.
func (r *ClinicalRepository) RecordByPatient(ctx context.Context, patientID string) (clinical.Record, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return clinical.Record{}, err
	}

	parsed, err := parseUUID(patientID, "patient_id")
	if err != nil {
		return clinical.Record{}, err
	}

	row, err := queries.GetClinicalRecordByPatient(ctx, parsed)
	if err != nil {
		return clinical.Record{}, translateClinicalError(err)
	}
	return recordToDomain(row)
}

// AddNote appends one encounter.
//
// The author arrives already resolved from the token; this adapter does not decide
// who wrote a note, it stores who the caller proved to be.
func (r *ClinicalRepository) AddNote(ctx context.Context, note clinical.Note) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	tenantID, err := toPGUUIDFromTenant(note.TenantID)
	if err != nil {
		return err
	}
	patientID, err := parseUUID(note.PatientID, "patient_id")
	if err != nil {
		return err
	}
	authorID, err := parseUUID(note.AuthorID, "author_user_id")
	if err != nil {
		return err
	}

	if _, err := queries.CreateClinicalNote(ctx, sqlcgen.CreateClinicalNoteParams{
		ID:            toNoteUUID(note.ID),
		TenantID:      tenantID,
		PatientID:     patientID,
		AuthorUserID:  authorID,
		EncounteredAt: note.EncounteredAt,
		Content:       note.Content,
		CreatedAt:     note.CreatedAt,
		UpdatedAt:     note.UpdatedAt,
	}); err != nil {
		return translateClinicalError(err)
	}
	return nil
}

// NotesByPatient returns one page of encounters, most recent first.
func (r *ClinicalRepository) NotesByPatient(ctx context.Context, patientID string, limit, offset int32) ([]clinical.Note, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}

	parsed, err := parseUUID(patientID, "patient_id")
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListClinicalNotesByPatient(ctx, sqlcgen.ListClinicalNotesByPatientParams{
		PatientID: parsed,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, translateClinicalError(err)
	}

	notes := make([]clinical.Note, 0, len(rows))
	for _, row := range rows {
		note, err := noteToDomain(row)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, nil
}

// recordToDomain rebuilds the frame from a row.
func recordToDomain(row sqlcgen.ClinicalRecord) (clinical.Record, error) {
	record, err := clinical.ReconstituteRecord(
		clinical.RecordID(row.ID.String()),
		tenantIDOf(row.TenantID),
		row.PatientID.String(),
		row.ChiefComplaint,
		row.Background,
		row.Objectives,
		row.CreatedAt,
		row.UpdatedAt,
	)
	if err != nil {
		return clinical.Record{}, fmt.Errorf("postgres: stored clinical record %s is invalid: %w", row.ID.String(), err)
	}
	return record, nil
}

// noteToDomain rebuilds a note from a row.
func noteToDomain(row sqlcgen.ClinicalNote) (clinical.Note, error) {
	note, err := clinical.ReconstituteNote(
		clinical.NoteID(row.ID.String()),
		tenantIDOf(row.TenantID),
		row.PatientID.String(),
		row.AuthorUserID.String(),
		row.EncounteredAt,
		row.Content,
		row.CreatedAt,
		row.UpdatedAt,
	)
	if err != nil {
		return clinical.Note{}, fmt.Errorf("postgres: stored clinical note %s is invalid: %w", row.ID.String(), err)
	}
	return note, nil
}

// translateClinicalError maps storage failures onto the clinical vocabulary.
func translateClinicalError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return clinical.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == foreignKeyViolationCode:
			// The composite keys of 0007: either the patient or the author is not
			// visible in this tenant. From the caller's side both mean the same.
			return fmt.Errorf("%w: constraint %s", clinical.ErrPatientNotVisible, pgErr.ConstraintName)
		case pgErr.Code == insufficientPrivilegeCode:
			return fmt.Errorf("%w: %s", clinical.ErrInsufficientPrivilege, pgErr.Message)
		}
	}

	return err
}

// parseUUID converts an identifier that must be a UUID, naming the field when it is
// not: a malformed id is a request problem, not a storage one.
func parseUUID(value, field string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, &clinical.ValidationError{
			Field:   field,
			Message: "must be a UUID",
		}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func toRecordUUID(id clinical.RecordID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func toNoteUUID(id clinical.NoteID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

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
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// patientsTenantDocumentUnique is the index behind per-tenant document uniqueness.
const patientsTenantDocumentUnique = "patients_tenant_document_unique"

// foreignKeyViolationCode is PostgreSQL's SQLSTATE for a broken reference. For this
// table it means the row names a document kind, tax kind or fiscal condition that
// the country does not have.
const foreignKeyViolationCode = "23503"

// PatientRepository is the PostgreSQL adapter for port.PatientRepository.
//
// Every method requires Transactor.WithTenant: row-level security derives the
// tenant from app.tenant_id, so the queries carry no tenant filter of their own.
// Called outside a tenant transaction the adapter returns
// patient.ErrTenantScopeMissing before touching the database.
type PatientRepository struct {
	queries *sqlcgen.Queries
}

// NewPatientRepository builds the adapter over an existing pool.
func NewPatientRepository(pool *pgxpool.Pool) *PatientRepository {
	return &PatientRepository{queries: sqlcgen.New(pool)}
}

// Compile-time proof that the adapter satisfies the port.
var _ port.PatientRepository = (*PatientRepository)(nil)

// scoped returns the query layer bound to the open tenant transaction, failing
// instead of falling back to the pool: a pooled connection carries no tenant
// setting, so a statement outside a transaction would be filtered into zero rows
// and look like an ordinary not-found.
func (r *PatientRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, patient.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Create inserts a new patient.
//
// The document arrives already normalized: the aggregate refuses a document
// without its canonical form, so the unique index compares what this writes and no
// caller can bypass it by typing separators.
func (r *PatientRepository) Create(ctx context.Context, subject patient.Patient) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	params, err := createPatientParams(subject)
	if err != nil {
		return err
	}
	if _, err := queries.CreatePatient(ctx, params); err != nil {
		return translatePatientError(err)
	}
	return nil
}

// ByID returns the patient visible in the current tenant, or patient.ErrNotFound.
func (r *PatientRepository) ByID(ctx context.Context, id patient.ID) (patient.Patient, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return patient.Patient{}, err
	}

	row, err := queries.GetPatientByID(ctx, toPatientUUID(id))
	if err != nil {
		return patient.Patient{}, translatePatientError(err)
	}
	return patientToDomain(row)
}

// List returns one page of patients, ordered by name.
func (r *PatientRepository) List(ctx context.Context, filter port.PatientFilter) ([]patient.Patient, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListPatients(ctx, sqlcgen.ListPatientsParams{
		Column1: filter.Search,
		Column2: filter.ActiveOnly,
		Column5: filter.OwnOnly,
		Column6: filter.ViewerID,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	})
	if err != nil {
		return nil, translatePatientError(err)
	}

	subjects := make([]patient.Patient, 0, len(rows))
	for _, row := range rows {
		subject, err := patientToDomain(row)
		if err != nil {
			return nil, err
		}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

// Update persists an edit. The document can change, and with it the unique index:
// a collision is patient.ErrDocumentTaken, not a driver error.
func (r *PatientRepository) Update(ctx context.Context, subject patient.Patient) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	_, err = queries.UpdatePatient(ctx, sqlcgen.UpdatePatientParams{
		ID:                    toPatientUUID(subject.ID),
		Country:               subject.Country,
		FullName:              subject.FullName,
		BirthDate:             toPGDate(subject.BirthDate),
		DocumentType:          documentField(subject.Document, func(d *patient.Document) string { return d.Kind }),
		DocumentNumber:        documentField(subject.Document, func(d *patient.Document) string { return d.Number }),
		DocumentNormalized:    documentField(subject.Document, func(d *patient.Document) string { return d.Normalized }),
		Phone:                 subject.Phone,
		Email:                 subject.Email,
		EmergencyContactName:  subject.EmergencyContactName,
		EmergencyContactPhone: subject.EmergencyContactPhone,
		TaxID:                 taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Number }),
		TaxIDKind:             taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Kind }),
		FiscalCondition:       subject.FiscalCondition,
		FiscalAddress:         subject.FiscalAddress,
		AdministrativeNotes:   subject.AdministrativeNotes,
		IsActive:              subject.IsActive,
		UpdatedAt:             subject.UpdatedAt,
	})
	if err != nil {
		return translatePatientError(err)
	}
	return nil
}

// AssignTreating sets the treating professional of an existing patient. It is a
// statement of its own, not part of Update, so the permission to edit a file and the
// permission to hand a patient to someone else stay separate in the database too.
func (r *PatientRepository) AssignTreating(ctx context.Context, subject patient.Patient) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}

	_, err = queries.SetPatientTreatingProfessional(ctx, sqlcgen.SetPatientTreatingProfessionalParams{
		ID:                     toPatientUUID(subject.ID),
		TreatingProfessionalID: optionalUUID(subject.TreatingProfessionalID),
		UpdatedAt:              subject.UpdatedAt,
	})
	if err != nil {
		return translatePatientError(err)
	}
	return nil
}

// createPatientParams maps the aggregate onto the insert.
func createPatientParams(subject patient.Patient) (sqlcgen.CreatePatientParams, error) {
	tenantID, err := toPGUUIDFromTenant(subject.TenantID)
	if err != nil {
		return sqlcgen.CreatePatientParams{}, err
	}

	return sqlcgen.CreatePatientParams{
		ID:                     toPatientUUID(subject.ID),
		TenantID:               tenantID,
		Country:                subject.Country,
		FullName:               subject.FullName,
		BirthDate:              toPGDate(subject.BirthDate),
		DocumentType:           documentField(subject.Document, func(d *patient.Document) string { return d.Kind }),
		DocumentNumber:         documentField(subject.Document, func(d *patient.Document) string { return d.Number }),
		DocumentNormalized:     documentField(subject.Document, func(d *patient.Document) string { return d.Normalized }),
		Phone:                  subject.Phone,
		Email:                  subject.Email,
		EmergencyContactName:   subject.EmergencyContactName,
		EmergencyContactPhone:  subject.EmergencyContactPhone,
		TaxID:                  taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Number }),
		TaxIDKind:              taxIDField(subject.TaxID, func(t *patient.TaxID) string { return t.Kind }),
		FiscalCondition:        subject.FiscalCondition,
		FiscalAddress:          subject.FiscalAddress,
		AdministrativeNotes:    subject.AdministrativeNotes,
		IsActive:               subject.IsActive,
		CreatedAt:              subject.CreatedAt,
		UpdatedAt:              subject.UpdatedAt,
		TreatingProfessionalID: optionalUUID(subject.TreatingProfessionalID),
	}, nil
}

// patientToDomain rebuilds the aggregate from a row.
func patientToDomain(row sqlcgen.Patient) (patient.Patient, error) {
	subject, err := patient.Reconstitute(patient.Stored{
		ID:                     patient.ID(row.ID.String()),
		TenantID:               tenantIDOf(row.TenantID),
		Country:                row.Country,
		FullName:               row.FullName,
		BirthDate:              dateFromPG(row.BirthDate),
		Document:               documentFromRow(row),
		Phone:                  row.Phone,
		Email:                  row.Email,
		EmergencyContactName:   row.EmergencyContactName,
		EmergencyContactPhone:  row.EmergencyContactPhone,
		TaxID:                  taxIDFromRow(row),
		FiscalCondition:        row.FiscalCondition,
		FiscalAddress:          row.FiscalAddress,
		AdministrativeNotes:    row.AdministrativeNotes,
		TreatingProfessionalID: uuidString(row.TreatingProfessionalID),
		IsActive:               row.IsActive,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	})
	if err != nil {
		return patient.Patient{}, fmt.Errorf("postgres: stored patient %s is invalid: %w", row.ID.String(), err)
	}
	return subject, nil
}

// documentFromRow rebuilds the document only when the row carries one: the schema
// allows a patient without a document, and an empty struct would look like one.
func documentFromRow(row sqlcgen.Patient) *patient.Document {
	if row.DocumentType == nil || row.DocumentNumber == nil || row.DocumentNormalized == nil {
		return nil
	}
	return &patient.Document{
		Country:    row.Country,
		Kind:       *row.DocumentType,
		Number:     *row.DocumentNumber,
		Normalized: *row.DocumentNormalized,
	}
}

// taxIDFromRow rebuilds the fiscal identifier only when both halves are present.
func taxIDFromRow(row sqlcgen.Patient) *patient.TaxID {
	if row.TaxID == nil || row.TaxIDKind == nil {
		return nil
	}
	return &patient.TaxID{
		Kind:       *row.TaxIDKind,
		Number:     *row.TaxID,
		Normalized: *row.TaxID,
	}
}

// documentField reads one column of an optional document.
func documentField(document *patient.Document, pick func(*patient.Document) string) *string {
	if document == nil {
		return nil
	}
	value := pick(document)
	return &value
}

// taxIDField reads one column of an optional fiscal identifier.
func taxIDField(taxID *patient.TaxID, pick func(*patient.TaxID) string) *string {
	if taxID == nil {
		return nil
	}
	value := pick(taxID)
	return &value
}

// translatePatientError maps storage failures onto the patient domain's vocabulary.
func translatePatientError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return patient.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == patientsTenantDocumentUnique:
			return fmt.Errorf("%w: constraint %s", patient.ErrDocumentTaken, pgErr.ConstraintName)
		case pgErr.Code == foreignKeyViolationCode:
			// The catalog says this country has no such kind, or no such fiscal
			// condition. Both are "the data does not match the country's rules".
			return fmt.Errorf("%w: %s", patient.ErrDocumentInvalid, pgErr.ConstraintName)
		case pgErr.Code == insufficientPrivilegeCode:
			return fmt.Errorf("%w: %s", patient.ErrInsufficientPrivilege, pgErr.Message)
		}
	}

	return err
}

// toPatientUUID converts the domain identity into the pgx type.
func toPatientUUID(id patient.ID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

// toPGDate maps an optional date onto the pgx type.
func toPGDate(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *value, Valid: true}
}

// dateFromPG maps a nullable date back onto the domain.
func dateFromPG(value pgtype.Date) *time.Time {
	if !value.Valid {
		return nil
	}
	when := value.Time
	return &when
}

// optionalUUID converts an optional professional identity into the pgx type; nil is
// the absent state, not the zero UUID.
func optionalUUID(value *string) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	parsed, err := uuid.Parse(*value)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

// uuidString maps a nullable column back onto the domain's string identity.
func uuidString(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := value.String()
	return &text
}

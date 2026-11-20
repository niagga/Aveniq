package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/billing"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

const (
	restrictViolationCode = "23001"

	vouchersPatientFK      = "vouchers_patient_same_tenant"
	vouchersAppointmentFK  = "vouchers_appointment_of_patient"
	vouchersIssuerFK       = "vouchers_issuer_same_tenant"
	vouchersVoiderFK       = "vouchers_voider_same_tenant"
	vouchersProfessionalFK = "vouchers_professional_same_tenant"

	// vouchersOneIssuedPerAppointment is the partial unique index of 0020: a session is billed
	// by one issued voucher at a time.
	vouchersOneIssuedPerAppointment = "vouchers_one_issued_per_appointment"
)

const defaultVoucherPage = int32(100)

// BillingRepository is the PostgreSQL adapter for port.BillingRepository.
//
// The consecutive number is not chosen in Go. NextVoucherNumber locks the
// practice's counter row inside the open tenant transaction; CreateVoucher
// uses that value. If the insert fails, the transaction rolls back and the
// number is never skipped.
type BillingRepository struct {
	queries *sqlcgen.Queries
}

// NewBillingRepository builds the adapter over an existing pool.
func NewBillingRepository(pool *pgxpool.Pool) *BillingRepository {
	return &BillingRepository{queries: sqlcgen.New(pool)}
}

var _ port.BillingRepository = (*BillingRepository)(nil)

func (r *BillingRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, billing.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Issue reserves the next number and writes the voucher.
func (r *BillingRepository) Issue(ctx context.Context, voucher billing.Voucher) (billing.Voucher, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Voucher{}, err
	}

	tenantID, err := toPGUUIDFromTenant(voucher.TenantID)
	if err != nil {
		return billing.Voucher{}, err
	}

	n, err := queries.NextVoucherNumber(ctx, tenantID)
	if err != nil {
		return billing.Voucher{}, translateBillingError(err)
	}
	numbered, err := voucher.AssignNumber(int(n))
	if err != nil {
		return billing.Voucher{}, err
	}

	params, err := createVoucherParams(numbered)
	if err != nil {
		return billing.Voucher{}, err
	}
	row, err := queries.CreateVoucher(ctx, params)
	if err != nil {
		return billing.Voucher{}, translateBillingError(err)
	}
	return voucherToDomain(row)
}

// Void records the void of an issued voucher. The number stays on the row.
func (r *BillingRepository) Void(ctx context.Context, voucher billing.Voucher) (billing.Voucher, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Voucher{}, err
	}
	if voucher.Status != billing.StatusVoided || voucher.VoidedAt == nil || voucher.VoidReason == nil {
		return billing.Voucher{}, billing.ErrImmutable
	}

	id, err := parseBillingUUID(string(voucher.ID), "id")
	if err != nil {
		return billing.Voucher{}, err
	}
	voidedBy, err := parseBillingUUID(voucher.VoidedBy, "voided_by")
	if err != nil {
		return billing.Voucher{}, err
	}

	row, err := queries.VoidVoucher(ctx, sqlcgen.VoidVoucherParams{
		ID:         id,
		VoidedBy:   voidedBy,
		VoidedAt:   pgtype.Timestamptz{Time: *voucher.VoidedAt, Valid: true},
		VoidReason: voucher.VoidReason,
		UpdatedAt:  voucher.UpdatedAt,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, getErr := queries.GetVoucher(ctx, id)
			if getErr != nil {
				return billing.Voucher{}, translateBillingError(getErr)
			}
			if existing.Status == string(billing.StatusVoided) {
				return billing.Voucher{}, billing.ErrAlreadyVoided
			}
			return billing.Voucher{}, billing.ErrImmutable
		}
		return billing.Voucher{}, translateBillingError(err)
	}
	return voucherToDomain(row)
}

// ByID returns one voucher, or billing.ErrNotFound.
func (r *BillingRepository) ByID(ctx context.Context, id billing.VoucherID) (billing.Voucher, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Voucher{}, err
	}
	parsed, err := parseBillingUUID(string(id), "id")
	if err != nil {
		return billing.Voucher{}, err
	}
	row, err := queries.GetVoucher(ctx, parsed)
	if err != nil {
		return billing.Voucher{}, translateBillingError(err)
	}
	return voucherToDomain(row)
}

// List returns one page of the practice's history, newest first.
func (r *BillingRepository) List(ctx context.Context, filter port.VoucherFilter) ([]billing.Voucher, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultVoucherPage
	}

	params := sqlcgen.ListVouchersParams{
		IssuedFrom: timestamptzFromTime(filter.From),
		IssuedTo:   timestamptzFromTime(filter.To),
		PatientID:  optionalID(filter.PatientID),
		Lim:        limit,
		Off:        filter.Offset,
	}
	if filter.Status != "" {
		status := filter.Status
		params.Status = &status
	}

	rows, err := queries.ListVouchers(ctx, params)
	if err != nil {
		return nil, translateBillingError(err)
	}
	out := make([]billing.Voucher, 0, len(rows))
	for _, row := range rows {
		item, err := voucherToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func createVoucherParams(voucher billing.Voucher) (sqlcgen.CreateVoucherParams, error) {
	id, err := parseBillingUUID(string(voucher.ID), "id")
	if err != nil {
		return sqlcgen.CreateVoucherParams{}, err
	}
	tenantID, err := toPGUUIDFromTenant(voucher.TenantID)
	if err != nil {
		return sqlcgen.CreateVoucherParams{}, err
	}
	issuedBy, err := parseBillingUUID(voucher.IssuedBy, "issued_by")
	if err != nil {
		return sqlcgen.CreateVoucherParams{}, err
	}
	insurerID, err := optionalBillingUUID(voucher.Photo.InsurerID, "insurer_id")
	if err != nil {
		return sqlcgen.CreateVoucherParams{}, err
	}
	professionalID, err := optionalBillingUUID(voucher.Photo.ProfessionalID, "professional_user_id")
	if err != nil {
		return sqlcgen.CreateVoucherParams{}, err
	}
	return sqlcgen.CreateVoucherParams{
		ID:                   id,
		TenantID:             tenantID,
		Number:               int32(voucher.Number),
		PatientID:            optionalID(voucher.PatientID),
		AppointmentID:        optionalID(voucher.AppointmentID),
		Concept:              voucher.Concept,
		AmountMinor:          voucher.AmountMinor,
		Currency:             voucher.Currency,
		PracticeName:         voucher.PracticeName,
		BuyerName:            voucher.BuyerName,
		BuyerTaxID:           voucher.BuyerTaxID,
		BuyerFiscalCondition: voucher.BuyerFiscalCondition,
		CoverageKind:         optionalText(string(voucher.Photo.Coverage)),
		InsurerID:            insurerID,
		InsurerName:          optionalText(voucher.Photo.InsurerName),
		Modality:             optionalText(voucher.Photo.Modality),
		ProfessionalUserID:   professionalID,
		ProfessionalName:     optionalText(voucher.Photo.ProfessionalName),
		Status:               string(voucher.Status),
		IssuedBy:             issuedBy,
		IssuedAt:             voucher.IssuedAt,
		CreatedAt:            voucher.CreatedAt,
		UpdatedAt:            voucher.UpdatedAt,
	}, nil
}

// optionalText maps an empty field to NULL.
func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// textOrEmpty reads a nullable text column as the domain's empty string.
func textOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// optionalBillingUUID maps an empty reference to NULL and rejects a malformed one.
func optionalBillingUUID(value, field string) (pgtype.UUID, error) {
	if value == "" {
		return pgtype.UUID{}, nil
	}
	return parseBillingUUID(value, field)
}

func voucherToDomain(row sqlcgen.Voucher) (billing.Voucher, error) {
	return billing.Reconstitute(
		billing.VoucherID(row.ID.String()),
		tenantIDOf(row.TenantID),
		int(row.Number),
		uuidText(row.PatientID),
		uuidText(row.AppointmentID),
		row.Concept,
		row.AmountMinor,
		row.Currency,
		row.PracticeName,
		row.BuyerName,
		row.BuyerTaxID,
		row.BuyerFiscalCondition,
		billing.Photo{
			Coverage:         billing.Coverage(textOrEmpty(row.CoverageKind)),
			InsurerID:        uuidText(row.InsurerID),
			InsurerName:      textOrEmpty(row.InsurerName),
			Modality:         textOrEmpty(row.Modality),
			ProfessionalID:   uuidText(row.ProfessionalUserID),
			ProfessionalName: textOrEmpty(row.ProfessionalName),
		},
		billing.Status(row.Status),
		row.IssuedBy.String(),
		row.IssuedAt,
		uuidText(row.VoidedBy),
		timestamptzFromPG(row.VoidedAt),
		row.VoidReason,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func translateBillingError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case foreignKeyViolationCode:
			switch pgErr.ConstraintName {
			case vouchersPatientFK:
				return fmt.Errorf("%w: constraint %s", billing.ErrPatientNotVisible, pgErr.ConstraintName)
			case vouchersAppointmentFK:
				return fmt.Errorf("%w: constraint %s", billing.ErrAppointmentNotVisible, pgErr.ConstraintName)
			case vouchersIssuerFK, vouchersVoiderFK:
				return fmt.Errorf("%w: constraint %s", billing.ErrActorNotVisible, pgErr.ConstraintName)
			case vouchersProfessionalFK:
				return fmt.Errorf("%w: constraint %s", billing.ErrProfessionalNotVisible, pgErr.ConstraintName)
			default:
				return fmt.Errorf("%w: constraint %s", billing.ErrPatientNotVisible, pgErr.ConstraintName)
			}
		case uniqueViolationCode:
			if pgErr.ConstraintName == vouchersOneIssuedPerAppointment {
				return fmt.Errorf("%w: constraint %s", billing.ErrAppointmentAlreadyBilled, pgErr.ConstraintName)
			}
		case restrictViolationCode:
			return fmt.Errorf("%w: %s", billing.ErrImmutable, pgErr.Message)
		case insufficientPrivilegeCode:
			return err
		}
	}
	return err
}

func parseBillingUUID(value, field string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, &billing.ValidationError{Field: field, Message: "must be a UUID"}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func optionalID(value string) pgtype.UUID {
	if value == "" {
		return pgtype.UUID{}
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

func uuidText(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}

func timestamptzFromTime(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

// PatientBilling returns how a patient of this practice is billed, or
// billing.ErrPatientNotVisible.
func (r *BillingRepository) PatientBilling(ctx context.Context, patientID string) (billing.PatientBilling, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.PatientBilling{}, err
	}
	parsed, err := parseBillingUUID(patientID, "patient_id")
	if err != nil {
		return billing.PatientBilling{}, err
	}
	row, err := queries.GetPatientBilling(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.PatientBilling{}, billing.ErrPatientNotVisible
	}
	if err != nil {
		return billing.PatientBilling{}, translateBillingError(err)
	}
	return billing.PatientBilling{
		Coverage:    billing.Coverage(row.CoverageKind),
		InsurerID:   uuidText(row.InsurerID),
		InsurerName: textOrEmpty(row.InsurerName),
		Currency:    row.BillingCurrency,
		Active:      row.IsActive,
	}, nil
}

// Appointment reads one session of this practice, or billing.ErrAppointmentNotVisible.
func (r *BillingRepository) Appointment(ctx context.Context, appointmentID string) (billing.Appointment, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Appointment{}, err
	}
	parsed, err := parseBillingUUID(appointmentID, "appointment_id")
	if err != nil {
		return billing.Appointment{}, err
	}
	row, err := queries.GetBillingAppointment(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.Appointment{}, billing.ErrAppointmentNotVisible
	}
	if err != nil {
		return billing.Appointment{}, translateBillingError(err)
	}
	return sessionOf(
		row.ID, row.PatientID, row.ProfessionalUserID, row.ProfessionalName,
		row.Modality, row.StartsAt, row.Status, row.BilledVoucherID,
	), nil
}

// PatientAppointments returns the newest sessions of one patient, bounded by limit.
func (r *BillingRepository) PatientAppointments(ctx context.Context, patientID string, limit int32) ([]billing.Appointment, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	parsed, err := parseBillingUUID(patientID, "patient_id")
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPatientAppointments(ctx, sqlcgen.ListPatientAppointmentsParams{
		PatientID: parsed,
		Lim:       limit,
	})
	if err != nil {
		return nil, translateBillingError(err)
	}
	out := make([]billing.Appointment, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionOf(
			row.ID, row.PatientID, row.ProfessionalUserID, row.ProfessionalName,
			row.Modality, row.StartsAt, row.Status, row.BilledVoucherID,
		))
	}
	return out, nil
}

// Professional returns an active professional of this practice, or
// billing.ErrProfessionalNotVisible.
func (r *BillingRepository) Professional(ctx context.Context, userID string) (billing.Professional, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Professional{}, err
	}
	parsed, err := parseBillingUUID(userID, "professional_user_id")
	if err != nil {
		return billing.Professional{}, err
	}
	row, err := queries.GetBillingProfessional(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.Professional{}, billing.ErrProfessionalNotVisible
	}
	if err != nil {
		return billing.Professional{}, translateBillingError(err)
	}
	return billing.Professional{ID: row.ID.String(), FullName: row.FullName}, nil
}

// Professionals lists the active professionals of this practice, by name.
func (r *BillingRepository) Professionals(ctx context.Context) ([]billing.Professional, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListBillingProfessionals(ctx)
	if err != nil {
		return nil, translateBillingError(err)
	}
	out := make([]billing.Professional, 0, len(rows))
	for _, row := range rows {
		out = append(out, billing.Professional{ID: row.ID.String(), FullName: row.FullName})
	}
	return out, nil
}

// sessionOf maps the session columns every appointment read returns.
func sessionOf(
	id, patientID, professionalID pgtype.UUID,
	professionalName, modality string,
	startsAt time.Time,
	status string,
	billedVoucherID pgtype.UUID,
) billing.Appointment {
	return billing.Appointment{
		ID:               id.String(),
		PatientID:        patientID.String(),
		ProfessionalID:   professionalID.String(),
		ProfessionalName: professionalName,
		Modality:         modality,
		StartsAt:         startsAt,
		Status:           status,
		BilledVoucherID:  uuidText(billedVoucherID),
	}
}

// Subject returns the billing identity of a patient of this practice, or
// billing.ErrPatientNotVisible: a patient of another practice is indistinguishable from
// one that does not exist.
func (r *BillingRepository) Subject(ctx context.Context, patientID string) (billing.Subject, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return billing.Subject{}, err
	}
	parsed, err := parseBillingUUID(patientID, "patient_id")
	if err != nil {
		return billing.Subject{}, err
	}
	row, err := queries.GetBillingSubject(ctx, parsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return billing.Subject{}, billing.ErrPatientNotVisible
	}
	if err != nil {
		return billing.Subject{}, translateBillingError(err)
	}
	return billing.Subject{
		PatientID:       row.ID.String(),
		FullName:        row.FullName,
		DocumentNumber:  row.DocumentNumber,
		TaxID:           row.TaxID,
		FiscalCondition: row.FiscalCondition,
		BillingCurrency: row.BillingCurrency,
	}, nil
}

// SearchSubjects returns the active patients a fragment matches, bounded by limit. The
// fragment is a literal: LIKE wildcards in it are escaped, so "%" finds nothing rather
// than everything. It matches the name as typed and the document or tax id without
// separators, so "30111222" finds a DNI saved as "30.111.222".
func (r *BillingRepository) SearchSubjects(ctx context.Context, fragment string, limit int32) ([]billing.Subject, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	// The identifier is NULL for a fragment too short to search documents by: the name
	// still matches, the roster of documents does not.
	var identifier *string
	if reduced := billing.IdentifierFragment(fragment); reduced != "" {
		identifier = &reduced
	}
	rows, err := queries.SearchBillingSubjects(ctx, sqlcgen.SearchBillingSubjectsParams{
		Pattern:    likePattern(fragment),
		Identifier: identifier,
		Lim:        limit,
	})
	if err != nil {
		return nil, translateBillingError(err)
	}
	out := make([]billing.Subject, 0, len(rows))
	for _, row := range rows {
		out = append(out, billing.Subject{
			PatientID:       row.ID.String(),
			FullName:        row.FullName,
			DocumentNumber:  row.DocumentNumber,
			TaxID:           row.TaxID,
			FiscalCondition: row.FiscalCondition,
			BillingCurrency: row.BillingCurrency,
		})
	}
	return out, nil
}

// likeEscaper escapes the LIKE metacharacters so a fragment is matched literally.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likePattern turns a literal fragment into a contains-pattern with its wildcards escaped.
func likePattern(fragment string) string {
	escaped := likeEscaper.Replace(fragment)
	return "%" + escaped + "%"
}

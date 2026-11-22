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
	"github.com/niagga/aveniq/backend/internal/domain/report"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

// Constraint names behind the saved-report errors the adapter translates.
const (
	savedReportsNameUnique          = "saved_reports_name_unique"
	savedReportsCreatorSameTenant   = "saved_reports_creator_same_tenant"
	savedReportsProfessionalSameTen = "saved_reports_professional_same_tenant"
	savedReportsInsurerSameTenant   = "saved_reports_insurer_same_tenant"

	savedReportDay = "2006-01-02"
)

// SavedReportRepository is the PostgreSQL adapter for port.SavedReportRepository.
//
// Like the tariff adapter it requires Transactor.WithTenant: row-level security derives the
// practice from app.tenant_id, so the queries carry no tenant filter.
type SavedReportRepository struct {
	queries *sqlcgen.Queries
}

// NewSavedReportRepository builds the adapter over an existing pool.
func NewSavedReportRepository(pool *pgxpool.Pool) *SavedReportRepository {
	return &SavedReportRepository{queries: sqlcgen.New(pool)}
}

var _ port.SavedReportRepository = (*SavedReportRepository)(nil)

func (r *SavedReportRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, report.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// Create stores a saved report.
func (r *SavedReportRepository) Create(ctx context.Context, saved report.SavedReport) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	tenantID, err := savedUUID(saved.TenantID, "tenant_id")
	if err != nil {
		return err
	}
	author, err := savedUUID(saved.CreatedBy, "created_by")
	if err != nil {
		return err
	}
	professional, insurerID, err := savedReferences(saved)
	if err != nil {
		return err
	}
	_, err = queries.CreateSavedReport(ctx, sqlcgen.CreateSavedReportParams{
		ID:                 toSavedID(saved.ID),
		TenantID:           tenantID,
		Name:               saved.Name,
		Period:             string(saved.Period),
		FromDay:            savedDay(saved.From),
		ToDay:              savedDay(saved.To),
		GroupBy:            string(saved.GroupBy),
		ProfessionalUserID: professional,
		Coverage:           optionalText(saved.Filter.Coverage),
		InsurerID:          insurerID,
		Modality:           optionalText(saved.Filter.Modality),
		CreatedBy:          author,
		CreatedAt:          saved.CreatedAt,
		UpdatedAt:          saved.UpdatedAt,
	})
	return translateSavedReportError(err)
}

// ByID reads one saved report of the practice, or report.ErrSavedNotFound.
func (r *SavedReportRepository) ByID(ctx context.Context, id report.SavedID) (report.SavedReport, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return report.SavedReport{}, err
	}
	row, err := queries.GetSavedReport(ctx, toSavedID(id))
	if err != nil {
		return report.SavedReport{}, translateSavedReportError(err)
	}
	return savedToDomain(row), nil
}

// List returns every saved report of the practice, by name, with the names the list shows.
func (r *SavedReportRepository) List(ctx context.Context) ([]port.SavedReportItem, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListSavedReports(ctx)
	if err != nil {
		return nil, translateSavedReportError(err)
	}
	items := make([]port.SavedReportItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, port.SavedReportItem{
			Report: savedToDomain(sqlcgen.SavedReport{
				ID: row.ID, TenantID: row.TenantID, Name: row.Name, Period: row.Period,
				FromDay: row.FromDay, ToDay: row.ToDay, GroupBy: row.GroupBy,
				ProfessionalUserID: row.ProfessionalUserID, Coverage: row.Coverage,
				InsurerID: row.InsurerID, Modality: row.Modality, CreatedBy: row.CreatedBy,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			}),
			CreatedByName:    row.CreatedByName,
			ProfessionalName: textOrEmpty(row.ProfessionalName),
			InsurerName:      textOrEmpty(row.InsurerName),
		})
	}
	return items, nil
}

// Update rewrites what a saved report asks. The author and the creation time are not written.
func (r *SavedReportRepository) Update(ctx context.Context, saved report.SavedReport) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	professional, insurerID, err := savedReferences(saved)
	if err != nil {
		return err
	}
	_, err = queries.UpdateSavedReport(ctx, sqlcgen.UpdateSavedReportParams{
		ID:                 toSavedID(saved.ID),
		Name:               saved.Name,
		Period:             string(saved.Period),
		FromDay:            savedDay(saved.From),
		ToDay:              savedDay(saved.To),
		GroupBy:            string(saved.GroupBy),
		ProfessionalUserID: professional,
		Coverage:           optionalText(saved.Filter.Coverage),
		InsurerID:          insurerID,
		Modality:           optionalText(saved.Filter.Modality),
		UpdatedAt:          saved.UpdatedAt,
	})
	return translateSavedReportError(err)
}

// Delete removes a saved report. One the practice does not hold is report.ErrSavedNotFound.
func (r *SavedReportRepository) Delete(ctx context.Context, id report.SavedID) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	removed, err := queries.DeleteSavedReport(ctx, toSavedID(id))
	if err != nil {
		return translateSavedReportError(err)
	}
	if removed == 0 {
		return report.ErrSavedNotFound
	}
	return nil
}

func savedToDomain(row sqlcgen.SavedReport) report.SavedReport {
	saved := report.SavedReport{
		ID:        report.SavedID(row.ID.String()),
		TenantID:  uuidText(row.TenantID),
		Name:      row.Name,
		Period:    report.Period(row.Period),
		GroupBy:   report.Dimension(row.GroupBy),
		CreatedBy: uuidText(row.CreatedBy),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		Filter: report.Filter{
			ProfessionalID: uuidText(row.ProfessionalUserID),
			Coverage:       textOrEmpty(row.Coverage),
			InsurerID:      uuidText(row.InsurerID),
			Modality:       textOrEmpty(row.Modality),
		},
	}
	if row.FromDay.Valid {
		saved.From = row.FromDay.Time.Format(savedReportDay)
	}
	if row.ToDay.Valid {
		saved.To = row.ToDay.Time.Format(savedReportDay)
	}
	return saved
}

// translateSavedReportError maps storage failures onto the report vocabulary.
func translateSavedReportError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return report.ErrSavedNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == uniqueViolationCode && pgErr.ConstraintName == savedReportsNameUnique:
			return fmt.Errorf("%w: constraint %s", report.ErrSavedNameTaken, pgErr.ConstraintName)
		case pgErr.Code == foreignKeyViolationCode && pgErr.ConstraintName == savedReportsCreatorSameTenant:
			return &report.ValidationError{Field: "created_by", Message: "must be a user of this practice"}
		case pgErr.Code == foreignKeyViolationCode && pgErr.ConstraintName == savedReportsProfessionalSameTen:
			return &report.ValidationError{Field: "professionalId", Message: "must be a user of this practice"}
		case pgErr.Code == foreignKeyViolationCode && pgErr.ConstraintName == savedReportsInsurerSameTenant:
			return &report.ValidationError{Field: "insurerId", Message: "must be a mutual of this practice"}
		}
	}
	return err
}

// savedReferences converts the two optional references of the filter.
func savedReferences(saved report.SavedReport) (professional, insurerID pgtype.UUID, err error) {
	if saved.Filter.ProfessionalID != "" {
		if professional, err = savedUUID(saved.Filter.ProfessionalID, "professionalId"); err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, err
		}
	}
	if saved.Filter.InsurerID != "" {
		if insurerID, err = savedUUID(saved.Filter.InsurerID, "insurerId"); err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, err
		}
	}
	return professional, insurerID, nil
}

// savedUUID converts a reference; one that is not a UUID is a field error.
func savedUUID(value, field string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, &report.ValidationError{Field: field, Message: "must be an identifier of this practice"}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

// toSavedID converts the identity. One that is not a UUID becomes the zero value, which matches
// no row: the caller sees not-found.
func toSavedID(id report.SavedID) pgtype.UUID {
	parsed, err := uuid.Parse(string(id))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}
}

// savedDay turns a civil day into a date; an empty one is NULL.
func savedDay(day string) pgtype.Date {
	if day == "" {
		return pgtype.Date{}
	}
	parsed, err := time.Parse(savedReportDay, day)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: parsed, Valid: true}
}

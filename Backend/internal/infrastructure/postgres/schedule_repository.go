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
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres/sqlcgen"
)

const exclusionViolationCode = "23P01"

// ScheduleRepository is the PostgreSQL adapter for port.ScheduleRepository.
type ScheduleRepository struct {
	queries *sqlcgen.Queries
}

// NewScheduleRepository builds the adapter over an existing pool.
func NewScheduleRepository(pool *pgxpool.Pool) *ScheduleRepository {
	return &ScheduleRepository{queries: sqlcgen.New(pool)}
}

var _ port.ScheduleRepository = (*ScheduleRepository)(nil)

func (r *ScheduleRepository) scoped(ctx context.Context) (*sqlcgen.Queries, error) {
	tx, ok := tenantTransaction(ctx)
	if !ok {
		return nil, schedule.ErrTenantScopeMissing
	}
	return r.queries.WithTx(tx), nil
}

// CreateAppointment inserts a slot. Overlap is 23P01 from appointments_no_overlap.
func (r *ScheduleRepository) CreateAppointment(ctx context.Context, slot schedule.Appointment) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	params, err := appointmentParams(slot)
	if err != nil {
		return err
	}
	_, err = queries.CreateAppointment(ctx, params)
	return translateScheduleError(err)
}

// AppointmentByID returns one slot, or schedule.ErrNotFound.
func (r *ScheduleRepository) AppointmentByID(ctx context.Context, id schedule.AppointmentID) (schedule.Appointment, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return schedule.Appointment{}, err
	}
	parsed, err := parseScheduleUUID(string(id), "id")
	if err != nil {
		return schedule.Appointment{}, err
	}
	row, err := queries.GetAppointment(ctx, parsed)
	if err != nil {
		return schedule.Appointment{}, translateScheduleError(err)
	}
	return appointmentToDomain(row)
}

// ListAppointments returns slots overlapping the window.
func (r *ScheduleRepository) ListAppointments(ctx context.Context, filter port.AppointmentFilter) ([]schedule.Appointment, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return nil, err
	}

	var rows []sqlcgen.Appointment
	if filter.ProfessionalID == "" {
		rows, err = queries.ListAppointmentsInRange(ctx, sqlcgen.ListAppointmentsInRangeParams{
			RangeFrom: filter.From,
			RangeTo:   filter.To,
		})
	} else {
		professionalID, parseErr := parseScheduleUUID(filter.ProfessionalID, "professional_user_id")
		if parseErr != nil {
			return nil, parseErr
		}
		rows, err = queries.ListAppointmentsInRangeForProfessional(ctx, sqlcgen.ListAppointmentsInRangeForProfessionalParams{
			ProfessionalUserID: professionalID,
			RangeFrom:          filter.From,
			RangeTo:            filter.To,
		})
	}
	if err != nil {
		return nil, translateScheduleError(err)
	}

	slots := make([]schedule.Appointment, 0, len(rows))
	for _, row := range rows {
		slot, err := appointmentToDomain(row)
		if err != nil {
			return nil, err
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

// SaveAppointment writes a status change.
func (r *ScheduleRepository) SaveAppointment(ctx context.Context, slot schedule.Appointment) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	parsed, err := parseScheduleUUID(string(slot.ID), "id")
	if err != nil {
		return err
	}
	_, err = queries.UpdateAppointmentStatus(ctx, sqlcgen.UpdateAppointmentStatusParams{
		ID:        parsed,
		Status:    string(slot.Status),
		UpdatedAt: slot.UpdatedAt,
	})
	return translateScheduleError(err)
}

// CreateSeries inserts the recurrence row.
func (r *ScheduleRepository) CreateSeries(ctx context.Context, series schedule.Series) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	params, err := seriesParams(series)
	if err != nil {
		return err
	}
	_, err = queries.CreateAppointmentSeries(ctx, params)
	return translateScheduleError(err)
}

// SeriesByID returns one series, or schedule.ErrNotFound.
func (r *ScheduleRepository) SeriesByID(ctx context.Context, id schedule.SeriesID) (schedule.Series, error) {
	queries, err := r.scoped(ctx)
	if err != nil {
		return schedule.Series{}, err
	}
	parsed, err := parseScheduleUUID(string(id), "id")
	if err != nil {
		return schedule.Series{}, err
	}
	row, err := queries.GetAppointmentSeries(ctx, parsed)
	if err != nil {
		return schedule.Series{}, translateScheduleError(err)
	}
	return seriesToDomain(row)
}

// SaveSeries updates the civil time of a series.
func (r *ScheduleRepository) SaveSeries(ctx context.Context, series schedule.Series) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	parsed, err := parseScheduleUUID(string(series.ID), "id")
	if err != nil {
		return err
	}
	_, err = queries.UpdateAppointmentSeries(ctx, sqlcgen.UpdateAppointmentSeriesParams{
		ID:               parsed,
		Weekday:          int16(series.Weekday),
		LocalStartMinute: int32(series.LocalStartMinute),
		DurationMinutes:  int32(series.DurationMinutes),
		StartsOn:         pgtype.Date{Time: series.StartsOn, Valid: true},
		UntilOn:          pgtype.Date{Time: series.UntilOn, Valid: true},
		UpdatedAt:        series.UpdatedAt,
	})
	return translateScheduleError(err)
}

// DeleteScheduledFrom removes future scheduled occurrences of a series.
func (r *ScheduleRepository) DeleteScheduledFrom(ctx context.Context, seriesID schedule.SeriesID, from time.Time) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	parsed, err := parseScheduleUUID(string(seriesID), "id")
	if err != nil {
		return err
	}
	return translateScheduleError(queries.DeleteScheduledOfSeriesFrom(ctx, sqlcgen.DeleteScheduledOfSeriesFromParams{
		SeriesID: pgtype.UUID{Bytes: parsed.Bytes, Valid: true},
		StartsAt: from,
	}))
}

// CancelScheduled cancels every still-scheduled occurrence of a series.
func (r *ScheduleRepository) CancelScheduled(ctx context.Context, seriesID schedule.SeriesID, at time.Time) error {
	queries, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	parsed, err := parseScheduleUUID(string(seriesID), "id")
	if err != nil {
		return err
	}
	return translateScheduleError(queries.CancelScheduledOfSeries(ctx, sqlcgen.CancelScheduledOfSeriesParams{
		SeriesID:  pgtype.UUID{Bytes: parsed.Bytes, Valid: true},
		UpdatedAt: at,
	}))
}

func appointmentParams(slot schedule.Appointment) (sqlcgen.CreateAppointmentParams, error) {
	id, err := parseScheduleUUID(string(slot.ID), "id")
	if err != nil {
		return sqlcgen.CreateAppointmentParams{}, err
	}
	tenantID, err := parseScheduleUUID(string(slot.TenantID), "tenant_id")
	if err != nil {
		return sqlcgen.CreateAppointmentParams{}, err
	}
	professionalID, err := parseScheduleUUID(slot.ProfessionalUserID, "professional_user_id")
	if err != nil {
		return sqlcgen.CreateAppointmentParams{}, err
	}
	patientID, err := parseScheduleUUID(slot.PatientID, "patient_id")
	if err != nil {
		return sqlcgen.CreateAppointmentParams{}, err
	}
	var seriesID pgtype.UUID
	if slot.SeriesID != "" {
		seriesID, err = parseScheduleUUID(slot.SeriesID, "series_id")
		if err != nil {
			return sqlcgen.CreateAppointmentParams{}, err
		}
	}
	return sqlcgen.CreateAppointmentParams{
		ID:                 id,
		TenantID:           tenantID,
		ProfessionalUserID: professionalID,
		PatientID:          patientID,
		SeriesID:           seriesID,
		StartsAt:           slot.StartsAt,
		EndsAt:             slot.EndsAt,
		Status:             string(slot.Status),
		Notes:              slot.Notes,
		CreatedAt:          slot.CreatedAt,
		UpdatedAt:          slot.UpdatedAt,
	}, nil
}

func seriesParams(series schedule.Series) (sqlcgen.CreateAppointmentSeriesParams, error) {
	id, err := parseScheduleUUID(string(series.ID), "id")
	if err != nil {
		return sqlcgen.CreateAppointmentSeriesParams{}, err
	}
	tenantID, err := parseScheduleUUID(string(series.TenantID), "tenant_id")
	if err != nil {
		return sqlcgen.CreateAppointmentSeriesParams{}, err
	}
	professionalID, err := parseScheduleUUID(series.ProfessionalUserID, "professional_user_id")
	if err != nil {
		return sqlcgen.CreateAppointmentSeriesParams{}, err
	}
	patientID, err := parseScheduleUUID(series.PatientID, "patient_id")
	if err != nil {
		return sqlcgen.CreateAppointmentSeriesParams{}, err
	}
	return sqlcgen.CreateAppointmentSeriesParams{
		ID:                 id,
		TenantID:           tenantID,
		ProfessionalUserID: professionalID,
		PatientID:          patientID,
		Weekday:            int16(series.Weekday),
		LocalStartMinute:   int32(series.LocalStartMinute),
		DurationMinutes:    int32(series.DurationMinutes),
		StartsOn:           pgtype.Date{Time: series.StartsOn, Valid: true},
		UntilOn:            pgtype.Date{Time: series.UntilOn, Valid: true},
		CreatedAt:          series.CreatedAt,
		UpdatedAt:          series.UpdatedAt,
	}, nil
}

func appointmentToDomain(row sqlcgen.Appointment) (schedule.Appointment, error) {
	seriesID := ""
	if row.SeriesID.Valid {
		seriesID = uuid.UUID(row.SeriesID.Bytes).String()
	}
	return schedule.Reconstitute(
		schedule.AppointmentID(row.ID.String()),
		tenant.ID(row.TenantID.String()),
		row.ProfessionalUserID.String(),
		row.PatientID.String(),
		seriesID,
		row.StartsAt,
		row.EndsAt,
		schedule.Status(row.Status),
		row.Notes,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func seriesToDomain(row sqlcgen.AppointmentSeries) (schedule.Series, error) {
	return schedule.ReconstituteSeries(
		schedule.SeriesID(row.ID.String()),
		tenant.ID(row.TenantID.String()),
		row.ProfessionalUserID.String(),
		row.PatientID.String(),
		int(row.Weekday),
		int(row.LocalStartMinute),
		int(row.DurationMinutes),
		row.StartsOn.Time,
		row.UntilOn.Time,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func translateScheduleError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return schedule.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case exclusionViolationCode:
			return schedule.ErrOverlap
		case foreignKeyViolationCode:
			if pgErr.ConstraintName == "appointments_professional_same_tenant" ||
				pgErr.ConstraintName == "appointment_series_professional_same_tenant" {
				return fmt.Errorf("%w: constraint %s", schedule.ErrProfessionalNotVisible, pgErr.ConstraintName)
			}
			return fmt.Errorf("%w: constraint %s", schedule.ErrPatientNotVisible, pgErr.ConstraintName)
		}
	}
	return err
}

func parseScheduleUUID(value, field string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, &schedule.ValidationError{Field: field, Message: "must be a UUID"}
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

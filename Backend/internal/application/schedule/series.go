package scheduleapp

import (
	"context"
	"time"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// SeriesInput is a weekly recurrence.
type SeriesInput struct {
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID           string
	ActorID            string
	Role               string
	ProfessionalUserID string
	PatientID          string
	Weekday            int
	LocalStartMinute   int
	DurationMinutes    int
	StartsOn           time.Time
	UntilOn            time.Time
}

// SeriesIDInput names a series.
type SeriesIDInput struct {
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// CreateSeriesUseCase materializes a weekly series.
type CreateSeriesUseCase struct {
	slots   port.ScheduleRepository
	tenants port.TenantRepository
	scope   port.TenantScope
	clock   port.Clock
	ids     port.IDGenerator
	access  *careapp.Access
}

// NewCreateSeriesUseCase wires the use case.
func NewCreateSeriesUseCase(
	slots port.ScheduleRepository,
	tenants port.TenantRepository,
	scope port.TenantScope,
	clock port.Clock,
	ids port.IDGenerator,
	access *careapp.Access,
) *CreateSeriesUseCase {
	return &CreateSeriesUseCase{slots: slots, tenants: tenants, scope: scope, clock: clock, ids: ids, access: access}
}

// Execute writes the series and every occurrence in one tenant transaction.
func (uc *CreateSeriesUseCase) Execute(ctx context.Context, in SeriesInput) (SeriesView, error) {
	practice, err := uc.tenants.ByID(ctx, tenant.ID(in.TenantID))
	if err != nil {
		return SeriesView{}, err
	}
	loc, err := time.LoadLocation(practice.Timezone)
	if err != nil {
		return SeriesView{}, &schedule.ValidationError{Field: "timezone", Message: "must be a valid IANA name"}
	}

	series, err := schedule.NewSeries(schedule.SeriesID(uc.ids.NewID()), tenant.ID(in.TenantID), schedule.SeriesDraft{
		ProfessionalUserID: in.ProfessionalUserID,
		PatientID:          in.PatientID,
		Weekday:            in.Weekday,
		LocalStartMinute:   in.LocalStartMinute,
		DurationMinutes:    in.DurationMinutes,
		StartsOn:           in.StartsOn,
		UntilOn:            in.UntilOn,
	}, uc.clock.Now())
	if err != nil {
		return SeriesView{}, err
	}

	intervals, err := series.Expand(loc)
	if err != nil {
		return SeriesView{}, err
	}

	created := make([]schedule.Appointment, 0, len(intervals))
	who := careapp.Viewer(in.ActorID, in.Role)
	if !care.ManagesAppointment(who, in.ProfessionalUserID) {
		return SeriesView{}, care.ErrForbidden
	}

	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		resolved, err := uc.access.Resolve(ctx, who, in.PatientID)
		if err != nil {
			return err
		}
		if !care.Books(who, resolved.Patient) {
			return care.ErrForbidden
		}
		if err := uc.slots.CreateSeries(ctx, series); err != nil {
			return err
		}
		for _, interval := range intervals {
			slot, err := schedule.NewAppointment(schedule.AppointmentID(uc.ids.NewID()), tenant.ID(in.TenantID), schedule.AppointmentDraft{
				ProfessionalUserID: in.ProfessionalUserID,
				PatientID:          in.PatientID,
				StartsAt:           interval.StartsAt,
				EndsAt:             interval.EndsAt,
			}, uc.clock.Now())
			if err != nil {
				return err
			}
			slot.SeriesID = string(series.ID)
			if err := uc.slots.CreateAppointment(ctx, slot); err != nil {
				return err
			}
			created = append(created, slot)
		}
		return nil
	})
	if err != nil {
		return SeriesView{}, err
	}
	return toSeriesView(series, created), nil
}

// UpdateSeriesUseCase rewrites future scheduled occurrences and leaves attended
// ones untouched.
type UpdateSeriesUseCase struct {
	slots   port.ScheduleRepository
	tenants port.TenantRepository
	scope   port.TenantScope
	clock   port.Clock
	ids     port.IDGenerator
}

// NewUpdateSeriesUseCase wires the use case.
func NewUpdateSeriesUseCase(
	slots port.ScheduleRepository,
	tenants port.TenantRepository,
	scope port.TenantScope,
	clock port.Clock,
	ids port.IDGenerator,
) *UpdateSeriesUseCase {
	return &UpdateSeriesUseCase{slots: slots, tenants: tenants, scope: scope, clock: clock, ids: ids}
}

// UpdateSeriesInput is a rewrite of an existing series' civil time.
type UpdateSeriesInput struct {
	TenantID         string
	ActorID          string
	Role             string
	ID               string
	Weekday          int
	LocalStartMinute int
	DurationMinutes  int
	StartsOn         time.Time
	UntilOn          time.Time
}

// Execute replaces future scheduled slots of the series.
func (uc *UpdateSeriesUseCase) Execute(ctx context.Context, in UpdateSeriesInput) (SeriesView, error) {
	practice, err := uc.tenants.ByID(ctx, tenant.ID(in.TenantID))
	if err != nil {
		return SeriesView{}, err
	}
	loc, err := time.LoadLocation(practice.Timezone)
	if err != nil {
		return SeriesView{}, &schedule.ValidationError{Field: "timezone", Message: "must be a valid IANA name"}
	}

	now := uc.clock.Now()
	var view SeriesView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		existing, err := uc.slots.SeriesByID(ctx, schedule.SeriesID(in.ID))
		if err != nil {
			return err
		}
		if !care.ManagesAppointment(careapp.Viewer(in.ActorID, in.Role), existing.ProfessionalUserID) {
			return schedule.ErrNotFound
		}

		updated, err := schedule.NewSeries(existing.ID, existing.TenantID, schedule.SeriesDraft{
			ProfessionalUserID: existing.ProfessionalUserID,
			PatientID:          existing.PatientID,
			Weekday:            in.Weekday,
			LocalStartMinute:   in.LocalStartMinute,
			DurationMinutes:    in.DurationMinutes,
			StartsOn:           in.StartsOn,
			UntilOn:            in.UntilOn,
		}, existing.CreatedAt)
		if err != nil {
			return err
		}
		updated.UpdatedAt = now.UTC()

		intervals, err := updated.Expand(loc)
		if err != nil {
			return err
		}

		if err := uc.slots.SaveSeries(ctx, updated); err != nil {
			return err
		}
		if err := uc.slots.DeleteScheduledFrom(ctx, updated.ID, now); err != nil {
			return err
		}

		created := make([]schedule.Appointment, 0, len(intervals))
		for _, interval := range intervals {
			if !interval.StartsAt.After(now) && !interval.StartsAt.Equal(now) {
				continue
			}
			slot, err := schedule.NewAppointment(schedule.AppointmentID(uc.ids.NewID()), updated.TenantID, schedule.AppointmentDraft{
				ProfessionalUserID: updated.ProfessionalUserID,
				PatientID:          updated.PatientID,
				StartsAt:           interval.StartsAt,
				EndsAt:             interval.EndsAt,
			}, now)
			if err != nil {
				return err
			}
			slot.SeriesID = string(updated.ID)
			if err := uc.slots.CreateAppointment(ctx, slot); err != nil {
				return err
			}
			created = append(created, slot)
		}
		view = toSeriesView(updated, created)
		return nil
	})
	if err != nil {
		return SeriesView{}, err
	}
	return view, nil
}

// CancelSeriesUseCase cancels every still-scheduled occurrence.
type CancelSeriesUseCase struct {
	slots port.ScheduleRepository
	scope port.TenantScope
	clock port.Clock
}

// NewCancelSeriesUseCase wires the use case.
func NewCancelSeriesUseCase(slots port.ScheduleRepository, scope port.TenantScope, clock port.Clock) *CancelSeriesUseCase {
	return &CancelSeriesUseCase{slots: slots, scope: scope, clock: clock}
}

// Execute cancels scheduled occurrences of the series.
func (uc *CancelSeriesUseCase) Execute(ctx context.Context, in SeriesIDInput) error {
	return uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		existing, err := uc.slots.SeriesByID(ctx, schedule.SeriesID(in.ID))
		if err != nil {
			return err
		}
		if !care.ManagesAppointment(careapp.Viewer(in.ActorID, in.Role), existing.ProfessionalUserID) {
			return schedule.ErrNotFound
		}
		return uc.slots.CancelScheduled(ctx, schedule.SeriesID(in.ID), uc.clock.Now())
	})
}

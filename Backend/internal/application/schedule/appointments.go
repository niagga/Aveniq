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

// CreateAppointmentInput is a one-off slot.
type CreateAppointmentInput struct {
	// TenantID, ActorID and Role come from the verified access token, never from the body.
	TenantID           string
	ActorID            string
	Role               string
	ProfessionalUserID string
	PatientID          string
	StartsAt           time.Time
	EndsAt             time.Time
	Notes              string
}

// ListAppointmentsInput is the weekly window.
type ListAppointmentsInput struct {
	TenantID       string
	ActorID        string
	Role           string
	From           time.Time
	To             time.Time
	ProfessionalID string
}

// TransitionInput names a slot whose status is changing.
type TransitionInput struct {
	TenantID string
	ActorID  string
	Role     string
	ID       string
}

// CreateAppointmentUseCase books a one-off slot.
//
// Two instance rules apply on top of the permission schedule:write: a professional
// books only in its own agenda and only with the patients it treats; the rest of the
// practice books with any patient. The patient must be one the caller may see.
type CreateAppointmentUseCase struct {
	slots  port.ScheduleRepository
	scope  port.TenantScope
	clock  port.Clock
	ids    port.IDGenerator
	access *careapp.Access
}

// NewCreateAppointmentUseCase wires the use case.
func NewCreateAppointmentUseCase(slots port.ScheduleRepository, scope port.TenantScope, clock port.Clock, ids port.IDGenerator, access *careapp.Access) *CreateAppointmentUseCase {
	return &CreateAppointmentUseCase{slots: slots, scope: scope, clock: clock, ids: ids, access: access}
}

// Execute inserts one scheduled slot. Overlap is refused by the database.
func (uc *CreateAppointmentUseCase) Execute(ctx context.Context, in CreateAppointmentInput) (AppointmentView, error) {
	notes := in.Notes
	slot, err := schedule.NewAppointment(
		schedule.AppointmentID(uc.ids.NewID()),
		tenant.ID(in.TenantID),
		schedule.AppointmentDraft{
			ProfessionalUserID: in.ProfessionalUserID,
			PatientID:          in.PatientID,
			StartsAt:           in.StartsAt,
			EndsAt:             in.EndsAt,
			Notes:              optional(notes),
		},
		uc.clock.Now(),
	)
	if err != nil {
		return AppointmentView{}, err
	}

	who := careapp.Viewer(in.ActorID, in.Role)
	if !care.ManagesAppointment(who, in.ProfessionalUserID) {
		return AppointmentView{}, care.ErrForbidden
	}

	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		resolved, err := uc.access.Resolve(ctx, who, in.PatientID)
		if err != nil {
			return err
		}
		if !care.Books(who, resolved.Patient) {
			return care.ErrForbidden
		}
		return uc.slots.CreateAppointment(ctx, slot)
	})
	if err != nil {
		return AppointmentView{}, err
	}
	return toAppointmentView(slot), nil
}

// ListAppointmentsUseCase reads the window the weekly screen shows.
//
// A professional's listing is its own agenda: asked without a professional it returns
// its own appointments, and asked for another professional it is refused.
type ListAppointmentsUseCase struct {
	slots port.ScheduleRepository
	scope port.TenantScope
}

// NewListAppointmentsUseCase wires the use case.
func NewListAppointmentsUseCase(slots port.ScheduleRepository, scope port.TenantScope) *ListAppointmentsUseCase {
	return &ListAppointmentsUseCase{slots: slots, scope: scope}
}

// Execute lists slots overlapping [From, To).
func (uc *ListAppointmentsUseCase) Execute(ctx context.Context, in ListAppointmentsInput) ([]AppointmentView, error) {
	if in.From.IsZero() || in.To.IsZero() || !in.To.After(in.From) {
		return nil, &schedule.ValidationError{Field: "range", Message: "to must be after from"}
	}

	who := careapp.Viewer(in.ActorID, in.Role)
	professionalID := in.ProfessionalID
	if care.OwnOnly(who) && professionalID == "" {
		professionalID = in.ActorID
	}
	if !care.ManagesAppointment(who, professionalID) {
		return nil, care.ErrForbidden
	}

	var views []AppointmentView
	err := uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		slots, err := uc.slots.ListAppointments(ctx, port.AppointmentFilter{
			From:           in.From.UTC(),
			To:             in.To.UTC(),
			ProfessionalID: professionalID,
		})
		if err != nil {
			return err
		}
		views = make([]AppointmentView, 0, len(slots))
		for _, slot := range slots {
			views = append(views, toAppointmentView(slot))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// AttendAppointmentUseCase marks a scheduled slot as having happened.
type AttendAppointmentUseCase struct {
	slots port.ScheduleRepository
	scope port.TenantScope
	clock port.Clock
}

// NewAttendAppointmentUseCase wires the use case.
func NewAttendAppointmentUseCase(slots port.ScheduleRepository, scope port.TenantScope, clock port.Clock) *AttendAppointmentUseCase {
	return &AttendAppointmentUseCase{slots: slots, scope: scope, clock: clock}
}

// Execute attends the slot.
func (uc *AttendAppointmentUseCase) Execute(ctx context.Context, in TransitionInput) (AppointmentView, error) {
	who := careapp.Viewer(in.ActorID, in.Role)
	return transition(ctx, uc.scope, in.TenantID, func(ctx context.Context) (schedule.Appointment, error) {
		slot, err := ownedSlot(ctx, uc.slots, who, in.ID)
		if err != nil {
			return schedule.Appointment{}, err
		}
		updated, err := slot.Attend(uc.clock.Now())
		if err != nil {
			return schedule.Appointment{}, err
		}
		if err := uc.slots.SaveAppointment(ctx, updated); err != nil {
			return schedule.Appointment{}, err
		}
		return updated, nil
	})
}

// CancelAppointmentUseCase frees a scheduled slot.
type CancelAppointmentUseCase struct {
	slots port.ScheduleRepository
	scope port.TenantScope
	clock port.Clock
}

// NewCancelAppointmentUseCase wires the use case.
func NewCancelAppointmentUseCase(slots port.ScheduleRepository, scope port.TenantScope, clock port.Clock) *CancelAppointmentUseCase {
	return &CancelAppointmentUseCase{slots: slots, scope: scope, clock: clock}
}

// Execute cancels the slot.
func (uc *CancelAppointmentUseCase) Execute(ctx context.Context, in TransitionInput) (AppointmentView, error) {
	who := careapp.Viewer(in.ActorID, in.Role)
	return transition(ctx, uc.scope, in.TenantID, func(ctx context.Context) (schedule.Appointment, error) {
		slot, err := ownedSlot(ctx, uc.slots, who, in.ID)
		if err != nil {
			return schedule.Appointment{}, err
		}
		updated, err := slot.Cancel(uc.clock.Now())
		if err != nil {
			return schedule.Appointment{}, err
		}
		if err := uc.slots.SaveAppointment(ctx, updated); err != nil {
			return schedule.Appointment{}, err
		}
		return updated, nil
	})
}

// ownedSlot loads the appointment the caller may work on. One that belongs to another
// professional answers schedule.ErrNotFound, the same as one that does not exist.
func ownedSlot(ctx context.Context, slots port.ScheduleRepository, who care.Viewer, id string) (schedule.Appointment, error) {
	slot, err := slots.AppointmentByID(ctx, schedule.AppointmentID(id))
	if err != nil {
		return schedule.Appointment{}, err
	}
	if !care.ManagesAppointment(who, slot.ProfessionalUserID) {
		return schedule.Appointment{}, schedule.ErrNotFound
	}
	return slot, nil
}

func transition(
	ctx context.Context,
	scope port.TenantScope,
	tenantID string,
	fn func(context.Context) (schedule.Appointment, error),
) (AppointmentView, error) {
	var view AppointmentView
	err := scope.WithTenant(ctx, tenantID, func(ctx context.Context) error {
		slot, err := fn(ctx)
		if err != nil {
			return err
		}
		view = toAppointmentView(slot)
		return nil
	})
	if err != nil {
		return AppointmentView{}, err
	}
	return view, nil
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

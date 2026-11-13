package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	scheduleapp "github.com/niagga/aveniq/backend/internal/application/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

type appointmentBody struct {
	ID                 string `json:"id,omitempty" format:"uuid"`
	ProfessionalUserID string `json:"professionalUserId,omitempty" format:"uuid"`
	PatientID          string `json:"patientId" format:"uuid"`
	SeriesID           string `json:"seriesId,omitempty" format:"uuid"`
	StartsAt           string `json:"startsAt" format:"date-time"`
	EndsAt             string `json:"endsAt" format:"date-time"`
	Status             string `json:"status,omitempty" enum:"scheduled,attended,cancelled"`
	Notes              string `json:"notes,omitempty"`
}

type seriesBody struct {
	ID                 string            `json:"id,omitempty" format:"uuid"`
	ProfessionalUserID string            `json:"professionalUserId,omitempty" format:"uuid"`
	PatientID          string            `json:"patientId" format:"uuid"`
	Weekday            int               `json:"weekday" minimum:"0" maximum:"6" doc:"0 is Sunday, matching Go and PostgreSQL."`
	LocalStartMinute   int               `json:"localStartMinute" minimum:"0" maximum:"1439" doc:"Minutes from local midnight in the practice timezone."`
	DurationMinutes    int               `json:"durationMinutes" minimum:"5" maximum:"480"`
	StartsOn           string            `json:"startsOn" format:"date"`
	UntilOn            string            `json:"untilOn" format:"date"`
	Appointments       []appointmentBody `json:"appointments,omitempty"`
}

func toAppointmentBody(view scheduleapp.AppointmentView) appointmentBody {
	return appointmentBody{
		ID:                 view.ID,
		ProfessionalUserID: view.ProfessionalUserID,
		PatientID:          view.PatientID,
		SeriesID:           view.SeriesID,
		StartsAt:           view.StartsAt,
		EndsAt:             view.EndsAt,
		Status:             view.Status,
		Notes:              view.Notes,
	}
}

func toSeriesBody(view scheduleapp.SeriesView) seriesBody {
	items := make([]appointmentBody, 0, len(view.Appointments))
	for _, slot := range view.Appointments {
		items = append(items, toAppointmentBody(slot))
	}
	return seriesBody{
		ID:                 view.ID,
		ProfessionalUserID: view.ProfessionalUserID,
		PatientID:          view.PatientID,
		Weekday:            view.Weekday,
		LocalStartMinute:   view.LocalStartMinute,
		DurationMinutes:    view.DurationMinutes,
		StartsOn:           view.StartsOn,
		UntilOn:            view.UntilOn,
		Appointments:       items,
	}
}

type scheduleHandlers struct {
	create       *scheduleapp.CreateAppointmentUseCase
	list         *scheduleapp.ListAppointmentsUseCase
	attend       *scheduleapp.AttendAppointmentUseCase
	cancel       *scheduleapp.CancelAppointmentUseCase
	createSeries *scheduleapp.CreateSeriesUseCase
	updateSeries *scheduleapp.UpdateSeriesUseCase
	cancelSeries *scheduleapp.CancelSeriesUseCase
	logger       *slog.Logger
}

func registerScheduleRoutes(api huma.API, deps Deps) {
	handlers := &scheduleHandlers{
		create:       deps.CreateAppointment,
		list:         deps.ListAppointments,
		attend:       deps.AttendAppointment,
		cancel:       deps.CancelAppointment,
		createSeries: deps.CreateSeries,
		updateSeries: deps.UpdateSeries,
		cancelSeries: deps.CancelSeries,
		logger:       deps.Logger,
	}

	guard := huma.Middlewares{
		requireRoles(api, deps.AccessTokens, iam.RoleOwner, iam.RoleAdmin, iam.RoleProfessional),
	}
	secured := []map[string][]string{{securitySchemeBearer: {}}}

	huma.Register(api, huma.Operation{
		OperationID:   "list-appointments",
		Method:        http.MethodGet,
		Path:          APIPrefix + "/appointments",
		Summary:       "List appointments in a window",
		Description:   "Returns slots that overlap [from, to). Filter by professional when the weekly view is showing one person.",
		Tags:          []string{"schedule"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.listHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "create-appointment",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/appointments",
		Summary:       "Book a one-off appointment",
		Tags:          []string{"schedule"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.createHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "attend-appointment",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/appointments/{id}/attend",
		Summary:       "Mark an appointment as attended",
		Tags:          []string{"schedule"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.attendHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "cancel-appointment",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/appointments/{id}/cancel",
		Summary:       "Cancel an appointment",
		Tags:          []string{"schedule"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.cancelHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "create-appointment-series",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/appointment-series",
		Summary:       "Create a weekly series",
		Description:   "Materializes concrete appointments in the practice timezone. Editing later does not rewrite attended slots.",
		Tags:          []string{"schedule"},
		DefaultStatus: http.StatusCreated,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.createSeriesHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "update-appointment-series",
		Method:        http.MethodPatch,
		Path:          APIPrefix + "/appointment-series/{id}",
		Summary:       "Rewrite future occurrences of a series",
		Tags:          []string{"schedule"},
		DefaultStatus: http.StatusOK,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.updateSeriesHandler)

	huma.Register(api, huma.Operation{
		OperationID:   "cancel-appointment-series",
		Method:        http.MethodPost,
		Path:          APIPrefix + "/appointment-series/{id}/cancel",
		Summary:       "Cancel remaining scheduled occurrences",
		Tags:          []string{"schedule"},
		DefaultStatus: http.StatusNoContent,
		Security:      secured,
		Middlewares:   guard,
	}, handlers.cancelSeriesHandler)
}

type listAppointmentsInput struct {
	From           string `query:"from" required:"true" format:"date-time" doc:"Window start, RFC 3339."`
	To             string `query:"to" required:"true" format:"date-time" doc:"Window end, RFC 3339."`
	ProfessionalID string `query:"professionalId" format:"uuid"`
}

type listAppointmentsOutput struct {
	Body struct {
		Items []appointmentBody `json:"items"`
	}
}

type appointmentIDInput struct {
	ID string `path:"id" format:"uuid"`
}

type appointmentOutput struct {
	Body appointmentBody
}

type createAppointmentInput struct {
	Body appointmentBody
}

type createSeriesInput struct {
	Body seriesBody
}

type updateSeriesInput struct {
	ID   string `path:"id" format:"uuid"`
	Body seriesBody
}

type seriesOutput struct {
	Body seriesBody
}

func (h *scheduleHandlers) listHandler(ctx context.Context, in *listAppointmentsInput) (*listAppointmentsOutput, error) {
	from, err := time.Parse(time.RFC3339, in.From)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("from must be an RFC 3339 timestamp")
	}
	to, err := time.Parse(time.RFC3339, in.To)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("to must be an RFC 3339 timestamp")
	}

	who := identityOf(ctx)
	views, err := h.list.Execute(ctx, scheduleapp.ListAppointmentsInput{
		TenantID:       who.TenantID,
		From:           from,
		To:             to,
		ProfessionalID: in.ProfessionalID,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}

	out := &listAppointmentsOutput{}
	out.Body.Items = make([]appointmentBody, 0, len(views))
	for _, view := range views {
		out.Body.Items = append(out.Body.Items, toAppointmentBody(view))
	}
	return out, nil
}

func (h *scheduleHandlers) createHandler(ctx context.Context, in *createAppointmentInput) (*appointmentOutput, error) {
	starts, err := time.Parse(time.RFC3339, in.Body.StartsAt)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("startsAt must be an RFC 3339 timestamp")
	}
	ends, err := time.Parse(time.RFC3339, in.Body.EndsAt)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("endsAt must be an RFC 3339 timestamp")
	}

	who := identityOf(ctx)
	professional := in.Body.ProfessionalUserID
	if professional == "" {
		professional = who.UserID
	}

	view, err := h.create.Execute(ctx, scheduleapp.CreateAppointmentInput{
		TenantID:           who.TenantID,
		ProfessionalUserID: professional,
		PatientID:          in.Body.PatientID,
		StartsAt:           starts,
		EndsAt:             ends,
		Notes:              in.Body.Notes,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	out := &appointmentOutput{}
	out.Body = toAppointmentBody(view)
	return out, nil
}

func (h *scheduleHandlers) attendHandler(ctx context.Context, in *appointmentIDInput) (*appointmentOutput, error) {
	who := identityOf(ctx)
	view, err := h.attend.Execute(ctx, scheduleapp.TransitionInput{TenantID: who.TenantID, ID: in.ID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	out := &appointmentOutput{}
	out.Body = toAppointmentBody(view)
	return out, nil
}

func (h *scheduleHandlers) cancelHandler(ctx context.Context, in *appointmentIDInput) (*appointmentOutput, error) {
	who := identityOf(ctx)
	view, err := h.cancel.Execute(ctx, scheduleapp.TransitionInput{TenantID: who.TenantID, ID: in.ID})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	out := &appointmentOutput{}
	out.Body = toAppointmentBody(view)
	return out, nil
}

func (h *scheduleHandlers) createSeriesHandler(ctx context.Context, in *createSeriesInput) (*seriesOutput, error) {
	startsOn, untilOn, err := parseSeriesDates(in.Body.StartsOn, in.Body.UntilOn)
	if err != nil {
		return nil, err
	}
	who := identityOf(ctx)
	professional := in.Body.ProfessionalUserID
	if professional == "" {
		professional = who.UserID
	}
	view, err := h.createSeries.Execute(ctx, scheduleapp.SeriesInput{
		TenantID:           who.TenantID,
		ProfessionalUserID: professional,
		PatientID:          in.Body.PatientID,
		Weekday:            in.Body.Weekday,
		LocalStartMinute:   in.Body.LocalStartMinute,
		DurationMinutes:    in.Body.DurationMinutes,
		StartsOn:           startsOn,
		UntilOn:            untilOn,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	out := &seriesOutput{}
	out.Body = toSeriesBody(view)
	return out, nil
}

func (h *scheduleHandlers) updateSeriesHandler(ctx context.Context, in *updateSeriesInput) (*seriesOutput, error) {
	startsOn, untilOn, err := parseSeriesDates(in.Body.StartsOn, in.Body.UntilOn)
	if err != nil {
		return nil, err
	}
	who := identityOf(ctx)
	view, err := h.updateSeries.Execute(ctx, scheduleapp.UpdateSeriesInput{
		TenantID:         who.TenantID,
		ID:               in.ID,
		Weekday:          in.Body.Weekday,
		LocalStartMinute: in.Body.LocalStartMinute,
		DurationMinutes:  in.Body.DurationMinutes,
		StartsOn:         startsOn,
		UntilOn:          untilOn,
	})
	if err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	out := &seriesOutput{}
	out.Body = toSeriesBody(view)
	return out, nil
}

func (h *scheduleHandlers) cancelSeriesHandler(ctx context.Context, in *appointmentIDInput) (*struct{}, error) {
	who := identityOf(ctx)
	if err := h.cancelSeries.Execute(ctx, scheduleapp.SeriesIDInput{TenantID: who.TenantID, ID: in.ID}); err != nil {
		return nil, problem(ctx, h.logger, err)
	}
	return nil, nil
}

func parseSeriesDates(startsOn, untilOn string) (time.Time, time.Time, error) {
	start, err := time.Parse("2006-01-02", startsOn)
	if err != nil {
		return time.Time{}, time.Time{}, huma.Error422UnprocessableEntity("startsOn must be a date in YYYY-MM-DD form")
	}
	until, err := time.Parse("2006-01-02", untilOn)
	if err != nil {
		return time.Time{}, time.Time{}, huma.Error422UnprocessableEntity("untilOn must be a date in YYYY-MM-DD form")
	}
	return start, until, nil
}

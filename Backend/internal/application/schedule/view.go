package scheduleapp

import (
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/schedule"
)

// AppointmentView is one slot as the API hands it out.
type AppointmentView struct {
	ID                 string
	ProfessionalUserID string
	PatientID          string
	SeriesID           string
	StartsAt           string
	EndsAt             string
	Status             string
	Notes              string
}

func toAppointmentView(slot schedule.Appointment) AppointmentView {
	notes := ""
	if slot.Notes != nil {
		notes = *slot.Notes
	}
	return AppointmentView{
		ID:                 string(slot.ID),
		ProfessionalUserID: slot.ProfessionalUserID,
		PatientID:          slot.PatientID,
		SeriesID:           slot.SeriesID,
		StartsAt:           slot.StartsAt.UTC().Format(time.RFC3339),
		EndsAt:             slot.EndsAt.UTC().Format(time.RFC3339),
		Status:             string(slot.Status),
		Notes:              notes,
	}
}

// SeriesView is the recurrence as the API hands it out.
type SeriesView struct {
	ID                 string
	ProfessionalUserID string
	PatientID          string
	Weekday            int
	LocalStartMinute   int
	DurationMinutes    int
	StartsOn           string
	UntilOn            string
	Appointments       []AppointmentView
}

func toSeriesView(series schedule.Series, slots []schedule.Appointment) SeriesView {
	views := make([]AppointmentView, 0, len(slots))
	for _, slot := range slots {
		views = append(views, toAppointmentView(slot))
	}
	return SeriesView{
		ID:                 string(series.ID),
		ProfessionalUserID: series.ProfessionalUserID,
		PatientID:          series.PatientID,
		Weekday:            int(series.Weekday),
		LocalStartMinute:   series.LocalStartMinute,
		DurationMinutes:    series.DurationMinutes,
		StartsOn:           series.StartsOn.Format("2006-01-02"),
		UntilOn:            series.UntilOn.Format("2006-01-02"),
		Appointments:       views,
	}
}

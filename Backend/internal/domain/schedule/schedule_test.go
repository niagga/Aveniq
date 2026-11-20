package schedule_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

func TestNewAppointmentRejectsInvertedInterval(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	_, err := schedule.NewAppointment("apt-1", "tenant-1", schedule.AppointmentDraft{
		ProfessionalUserID: "pro-1",
		PatientID:          "pat-1",
		StartsAt:           now,
		EndsAt:             now,
		Modality:           schedule.ModalityPresencial,
	}, now)
	if err == nil {
		t.Fatal("an empty interval must be refused")
	}
}

func TestNewAppointmentRequiresAKnownModality(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	draft := schedule.AppointmentDraft{
		ProfessionalUserID: "pro-1",
		PatientID:          "pat-1",
		StartsAt:           now,
		EndsAt:             now.Add(45 * time.Minute),
	}

	for _, modality := range []schedule.Modality{"", "hybrid", "Online"} {
		draft.Modality = modality
		if _, err := schedule.NewAppointment("apt-1", tenant.ID("tenant-1"), draft, now); err == nil {
			t.Fatalf("modality %q must be refused", modality)
		}
	}

	for _, modality := range []schedule.Modality{schedule.ModalityOnline, schedule.ModalityPresencial} {
		draft.Modality = modality
		slot, err := schedule.NewAppointment("apt-1", tenant.ID("tenant-1"), draft, now)
		if err != nil {
			t.Fatalf("modality %q: %v", modality, err)
		}
		if slot.Modality != modality {
			t.Fatalf("slot modality = %q, want %q", slot.Modality, modality)
		}
	}
}

func TestAttendThenCancelIsRefused(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	slot, err := schedule.NewAppointment("apt-1", tenant.ID("tenant-1"), schedule.AppointmentDraft{
		ProfessionalUserID: "pro-1",
		PatientID:          "pat-1",
		StartsAt:           now,
		EndsAt:             now.Add(45 * time.Minute),
		Modality:           schedule.ModalityPresencial,
	}, now)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	attended, err := slot.Attend(now)
	if err != nil {
		t.Fatalf("attend: %v", err)
	}
	if _, err := attended.Cancel(now); !errors.Is(err, schedule.ErrInvalidTransition) {
		t.Fatalf("cancel attended: error = %v, want %v", err, schedule.ErrInvalidTransition)
	}
}

func TestNewSeriesRequiresAKnownModality(t *testing.T) {
	draft := schedule.SeriesDraft{
		ProfessionalUserID: "pro-1",
		PatientID:          "pat-1",
		Weekday:            int(time.Tuesday),
		LocalStartMinute:   18 * 60,
		DurationMinutes:    45,
		StartsOn:           time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
		UntilOn:            time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC),
	}
	if _, err := schedule.NewSeries("ser-1", "tenant-1", draft, time.Now()); err == nil {
		t.Fatal("a series without modality must be refused")
	}

	draft.Modality = schedule.ModalityPresencial
	series, err := schedule.NewSeries("ser-1", "tenant-1", draft, time.Now())
	if err != nil {
		t.Fatalf("new series: %v", err)
	}
	if series.Modality != schedule.ModalityPresencial {
		t.Fatalf("series modality = %q, want presencial", series.Modality)
	}
}

func TestExpandWeeklyUsesTenantTimezone(t *testing.T) {
	buenosAires, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatal(err)
	}

	series, err := schedule.NewSeries("ser-1", "tenant-1", schedule.SeriesDraft{
		ProfessionalUserID: "pro-1",
		PatientID:          "pat-1",
		Weekday:            int(time.Tuesday),
		LocalStartMinute:   18 * 60,
		DurationMinutes:    45,
		Modality:           schedule.ModalityOnline,
		StartsOn:           time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),  // Sunday
		UntilOn:            time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC), // three Tuesdays: 10, 17, 24
	}, time.Now())
	if err != nil {
		t.Fatalf("new series: %v", err)
	}

	intervals, err := series.Expand(buenosAires)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(intervals) != 3 {
		t.Fatalf("got %d intervals, want 3", len(intervals))
	}

	first := intervals[0].StartsAt.In(buenosAires)
	if first.Weekday() != time.Tuesday || first.Hour() != 18 {
		t.Fatalf("first occurrence = %v, want Tuesday 18:00 in Buenos Aires", first)
	}
	if intervals[0].EndsAt.Sub(intervals[0].StartsAt) != 45*time.Minute {
		t.Fatalf("duration = %s, want 45m", intervals[0].EndsAt.Sub(intervals[0].StartsAt))
	}
}

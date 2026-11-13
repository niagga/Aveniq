package scheduleapp_test

import (
	"context"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	scheduleapp "github.com/niagga/aveniq/backend/internal/application/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

type fakeScope struct{}

func (fakeScope) WithTenant(ctx context.Context, tenantID string, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeIDs struct{ n int }

func (i *fakeIDs) NewID() string {
	i.n++
	return "00000000-0000-0000-0000-00000000000" + string(rune('0'+i.n))
}

type fakeSlots struct {
	created []schedule.Appointment
}

func (f *fakeSlots) CreateAppointment(_ context.Context, slot schedule.Appointment) error {
	f.created = append(f.created, slot)
	return nil
}
func (f *fakeSlots) AppointmentByID(context.Context, schedule.AppointmentID) (schedule.Appointment, error) {
	return schedule.Appointment{}, schedule.ErrNotFound
}
func (f *fakeSlots) ListAppointments(context.Context, port.AppointmentFilter) ([]schedule.Appointment, error) {
	return f.created, nil
}
func (f *fakeSlots) SaveAppointment(context.Context, schedule.Appointment) error { return nil }
func (f *fakeSlots) CreateSeries(context.Context, schedule.Series) error         { return nil }
func (f *fakeSlots) SeriesByID(context.Context, schedule.SeriesID) (schedule.Series, error) {
	return schedule.Series{}, schedule.ErrNotFound
}
func (f *fakeSlots) SaveSeries(context.Context, schedule.Series) error { return nil }
func (f *fakeSlots) DeleteScheduledFrom(context.Context, schedule.SeriesID, time.Time) error {
	return nil
}
func (f *fakeSlots) CancelScheduled(context.Context, schedule.SeriesID, time.Time) error {
	return nil
}

type fakeTenants struct {
	row tenant.Tenant
}

func (f fakeTenants) Create(context.Context, tenant.Tenant) error { return nil }
func (f fakeTenants) BySlug(context.Context, string) (tenant.Tenant, error) {
	return tenant.Tenant{}, tenant.ErrNotFound
}
func (f fakeTenants) ByID(context.Context, tenant.ID) (tenant.Tenant, error) { return f.row, nil }
func (f fakeTenants) List(context.Context, int32, int32) ([]tenant.Tenant, error) {
	return nil, nil
}

func TestCreateAppointmentPersistsAScheduledSlot(t *testing.T) {
	now := time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)
	slots := &fakeSlots{}
	uc := scheduleapp.NewCreateAppointmentUseCase(slots, fakeScope{}, fakeClock{now}, &fakeIDs{})

	view, err := uc.Execute(context.Background(), scheduleapp.CreateAppointmentInput{
		TenantID:           "11111111-1111-1111-1111-111111111111",
		ProfessionalUserID: "22222222-2222-2222-2222-222222222222",
		PatientID:          "33333333-3333-3333-3333-333333333333",
		StartsAt:           now,
		EndsAt:             now.Add(45 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if view.Status != "scheduled" {
		t.Fatalf("status = %s", view.Status)
	}
	if len(slots.created) != 1 {
		t.Fatalf("created %d slots", len(slots.created))
	}
}

func TestCreateSeriesExpandsWeeklyOccurrences(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	practice, err := tenant.New("11111111-1111-1111-1111-111111111111", "demo-sur", "Demo", now)
	if err != nil {
		t.Fatal(err)
	}
	slots := &fakeSlots{}
	uc := scheduleapp.NewCreateSeriesUseCase(slots, fakeTenants{row: practice}, fakeScope{}, fakeClock{now}, &fakeIDs{})

	view, err := uc.Execute(context.Background(), scheduleapp.SeriesInput{
		TenantID:           string(practice.ID),
		ProfessionalUserID: "22222222-2222-2222-2222-222222222222",
		PatientID:          "33333333-3333-3333-3333-333333333333",
		Weekday:            int(time.Monday),
		LocalStartMinute:   10 * 60,
		DurationMinutes:    45,
		StartsOn:           time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), // Monday
		UntilOn:            time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(view.Appointments) != 3 {
		t.Fatalf("got %d appointments, want 3 Mondays", len(view.Appointments))
	}
}

package scheduleapp_test

import (
	"context"
	"testing"
	"time"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	scheduleapp "github.com/niagga/aveniq/backend/internal/application/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// newAccess builds the access helper over a patient that exists and nobody treats. These tests run as an
// unrestricted caller, so the access helper only has to find the patient.
func newAccess() *careapp.Access {
	return careapp.NewAccess(fakePatients{}, fakeGrants{})
}

type fakePatients struct{}

func (fakePatients) Create(context.Context, patient.Patient) error { return nil }
func (fakePatients) ByID(_ context.Context, id patient.ID) (patient.Patient, error) {
	return patient.Patient{ID: id, IsActive: true, FullName: "Ana"}, nil
}
func (fakePatients) List(context.Context, port.PatientFilter) ([]patient.Patient, error) {
	return nil, nil
}
func (fakePatients) Update(context.Context, patient.Patient) error         { return nil }
func (fakePatients) AssignTreating(context.Context, patient.Patient) error { return nil }

type fakeGrants struct{}

func (fakeGrants) Create(context.Context, care.Grant) error { return nil }
func (fakeGrants) ByID(context.Context, string) (care.Grant, error) {
	return care.Grant{}, care.ErrGrantNotFound
}
func (fakeGrants) HasLiveGrant(context.Context, string, string) (bool, error)  { return false, nil }
func (fakeGrants) ListByPatient(context.Context, string) ([]care.Grant, error) { return nil, nil }
func (fakeGrants) Save(context.Context, care.Grant) error                      { return nil }

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
	byID    map[schedule.AppointmentID]schedule.Appointment
}

func (f *fakeSlots) store(slot schedule.Appointment) {
	if f.byID == nil {
		f.byID = map[schedule.AppointmentID]schedule.Appointment{}
	}
	f.byID[slot.ID] = slot
}

func (f *fakeSlots) CreateAppointment(_ context.Context, slot schedule.Appointment) error {
	f.created = append(f.created, slot)
	f.store(slot)
	return nil
}
func (f *fakeSlots) AppointmentByID(_ context.Context, id schedule.AppointmentID) (schedule.Appointment, error) {
	if slot, ok := f.byID[id]; ok {
		return slot, nil
	}
	return schedule.Appointment{}, schedule.ErrNotFound
}
func (f *fakeSlots) ListAppointments(context.Context, port.AppointmentFilter) ([]schedule.Appointment, error) {
	return f.created, nil
}
func (f *fakeSlots) SaveAppointment(_ context.Context, slot schedule.Appointment) error {
	f.store(slot)
	return nil
}
func (f *fakeSlots) RescheduleAppointment(_ context.Context, slot schedule.Appointment) error {
	f.store(slot)
	return nil
}
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

type fakeAudit struct {
	events []port.AuditEvent
}

func (f *fakeAudit) Record(_ context.Context, event port.AuditEvent) error {
	f.events = append(f.events, event)
	return nil
}

func TestRescheduleWritesAuditTrail(t *testing.T) {
	now := time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)
	slots := &fakeSlots{}
	audit := &fakeAudit{}
	slot, err := schedule.NewAppointment(
		schedule.AppointmentID("11111111-1111-1111-1111-111111111111"),
		tenant.ID("22222222-2222-2222-2222-222222222222"),
		schedule.AppointmentDraft{
			ProfessionalUserID: "33333333-3333-3333-3333-333333333333",
			PatientID:          "44444444-4444-4444-4444-444444444444",
			StartsAt:           now,
			EndsAt:             now.Add(45 * time.Minute),
			Modality:           schedule.ModalityPresencial,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	slots.store(slot)

	uc := scheduleapp.NewRescheduleAppointmentUseCase(slots, fakeScope{}, fakeClock{now}, audit)
	later := now.Add(24 * time.Hour)
	view, err := uc.Execute(context.Background(), scheduleapp.RescheduleAppointmentInput{
		TenantID: "22222222-2222-2222-2222-222222222222",
		ActorID:  "33333333-3333-3333-3333-333333333333",
		Role:     "owner",
		ID:       string(slot.ID),
		StartsAt: later,
		EndsAt:   later.Add(45 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	wantStart := later.UTC().Format(time.RFC3339)
	if view.StartsAt != wantStart {
		t.Fatalf("starts = %s, want %s", view.StartsAt, wantStart)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "appointment.rescheduled" {
		t.Fatalf("audit = %+v, want appointment.rescheduled", audit.events)
	}
}

func TestCreateAppointmentPersistsAScheduledSlot(t *testing.T) {
	now := time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)
	slots := &fakeSlots{}
	uc := scheduleapp.NewCreateAppointmentUseCase(slots, fakeScope{}, fakeClock{now}, &fakeIDs{}, newAccess())

	view, err := uc.Execute(context.Background(), scheduleapp.CreateAppointmentInput{
		TenantID:           "11111111-1111-1111-1111-111111111111",
		ProfessionalUserID: "22222222-2222-2222-2222-222222222222",
		PatientID:          "33333333-3333-3333-3333-333333333333",
		StartsAt:           now,
		EndsAt:             now.Add(45 * time.Minute),
		Modality:           "online",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if view.Status != "scheduled" {
		t.Fatalf("status = %s", view.Status)
	}
	if view.Modality != "online" {
		t.Fatalf("modality = %s, want online", view.Modality)
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
	uc := scheduleapp.NewCreateSeriesUseCase(slots, fakeTenants{row: practice}, fakeScope{}, fakeClock{now}, &fakeIDs{}, newAccess())

	view, err := uc.Execute(context.Background(), scheduleapp.SeriesInput{
		TenantID:           string(practice.ID),
		ProfessionalUserID: "22222222-2222-2222-2222-222222222222",
		PatientID:          "33333333-3333-3333-3333-333333333333",
		Weekday:            int(time.Monday),
		LocalStartMinute:   10 * 60,
		DurationMinutes:    45,
		Modality:           "presencial",
		StartsOn:           time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), // Monday
		UntilOn:            time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(view.Appointments) != 3 {
		t.Fatalf("got %d appointments, want 3 Mondays", len(view.Appointments))
	}
	if view.Modality != "presencial" {
		t.Fatalf("series modality = %s, want presencial", view.Modality)
	}
	for _, slot := range view.Appointments {
		if slot.Modality != "presencial" {
			t.Fatalf("occurrence %s modality = %s, want presencial", slot.StartsAt, slot.Modality)
		}
	}
}

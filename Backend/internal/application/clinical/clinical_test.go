package clinicalapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	clinicalapp "github.com/niagga/aveniq/backend/internal/application/clinical"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// The audit trail is the reason this package has tests of its own: "a read that cannot
// be audited must not succeed" is a decision, and a decision that only lives in a code
// path is a hope. The instance rules are tested here too, because they decide whether
// the read happens at all. These run without a database.

type fakeScope struct{ opened int }

func (s *fakeScope) WithTenant(ctx context.Context, _ string, fn func(context.Context) error) error {
	s.opened++
	return fn(ctx)
}

type fakeRepository struct {
	record    *clinical.Record
	notes     []clinical.Note
	recordErr error
}

func (r *fakeRepository) UpsertRecord(_ context.Context, record clinical.Record) (clinical.Record, error) {
	r.record = &record
	return record, nil
}

func (r *fakeRepository) RecordByPatient(context.Context, string) (clinical.Record, error) {
	if r.recordErr != nil {
		return clinical.Record{}, r.recordErr
	}
	if r.record == nil {
		return clinical.Record{}, clinical.ErrNotFound
	}
	return *r.record, nil
}

func (r *fakeRepository) AddNote(_ context.Context, note clinical.Note) error {
	r.notes = append(r.notes, note)
	return nil
}

func (r *fakeRepository) NotesByPatient(context.Context, string, int32, int32) ([]clinical.Note, error) {
	return r.notes, nil
}

type fakeAudit struct {
	events []port.AuditEvent
	err    error
}

func (a *fakeAudit) Record(_ context.Context, event port.AuditEvent) error {
	if a.err != nil {
		return a.err
	}
	a.events = append(a.events, event)
	return nil
}

// fakePatients serves one patient, whatever id is asked for.
type fakePatients struct{ subject patient.Patient }

func (f fakePatients) Create(context.Context, patient.Patient) error { return nil }
func (f fakePatients) ByID(context.Context, patient.ID) (patient.Patient, error) {
	return f.subject, nil
}
func (f fakePatients) List(context.Context, port.PatientFilter) ([]patient.Patient, error) {
	return nil, nil
}
func (f fakePatients) Update(context.Context, patient.Patient) error         { return nil }
func (f fakePatients) AssignTreating(context.Context, patient.Patient) error { return nil }

// fakeGrants answers the one question the read paths ask.
type fakeGrants struct{ live bool }

func (g fakeGrants) Create(context.Context, care.Grant) error { return nil }
func (g fakeGrants) ByID(context.Context, string) (care.Grant, error) {
	return care.Grant{}, care.ErrGrantNotFound
}
func (g fakeGrants) HasLiveGrant(context.Context, string, string) (bool, error) {
	return g.live, nil
}
func (g fakeGrants) ListByPatient(context.Context, string) ([]care.Grant, error) { return nil, nil }
func (g fakeGrants) Save(context.Context, care.Grant) error                       { return nil }

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type fixedIDs struct{}

func (fixedIDs) NewID() string { return uuid.NewString() }

var now = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

// accessFor builds the access helper over one patient, treated by treatingID, with
// or without a live grant for the caller.
func accessFor(patientID, treatingID string, granted bool) *careapp.Access {
	subject := patient.Patient{ID: patient.ID(patientID), TenantID: tenant.ID(uuid.NewString()), FullName: "Ana", IsActive: true}
	subject = subject.WithTreating(treatingID)
	return careapp.NewAccess(fakePatients{subject: subject}, fakeGrants{live: granted})
}

func existingRecord(t *testing.T) clinical.Record {
	t.Helper()

	complaint := "Insomnio"
	record, err := clinical.NewRecord(
		clinical.RecordID(uuid.NewString()), tenant.ID(uuid.NewString()), uuid.NewString(),
		clinical.RecordDraft{ChiefComplaint: &complaint}, now,
	)
	if err != nil {
		t.Fatalf("cannot build the record: %v", err)
	}
	return record
}

func TestReadingClinicalDataIsAudited(t *testing.T) {
	record := existingRecord(t)
	repo := &fakeRepository{record: &record}
	audit := &fakeAudit{}
	scope := &fakeScope{}
	actor := uuid.NewString()

	uc := clinicalapp.NewGetRecordUseCase(repo, scope, audit, accessFor(record.PatientID, actor, false))

	view, err := uc.Execute(context.Background(), clinicalapp.RecordInput{
		TenantID:  string(record.TenantID),
		ActorID:   actor,
		Role:      string(iam.RoleProfessional),
		PatientID: record.PatientID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.ChiefComplaint != "Insomnio" {
		t.Errorf("ChiefComplaint = %q, want the stored value", view.ChiefComplaint)
	}

	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want exactly one", len(audit.events))
	}
	event := audit.events[0]
	if event.Action != "clinical.read" || event.EntityID != record.PatientID {
		t.Errorf("event = %+v, want a clinical.read of this patient", event)
	}
	if event.TenantID != string(record.TenantID) {
		t.Errorf("event tenant = %q, want the caller's practice", event.TenantID)
	}
	if scope.opened != 1 {
		t.Errorf("tenant scopes opened = %d, want 1: the audit write shares the read transaction", scope.opened)
	}
}

// TestReadFailsWhenTheAuditFails is the fail-closed decision made explicit. Without it,
// the same code would return clinical data and a swallowed error, which is the one
// outcome the audit trail exists to prevent.
func TestReadFailsWhenTheAuditFails(t *testing.T) {
	record := existingRecord(t)
	repo := &fakeRepository{record: &record}
	audit := &fakeAudit{err: errors.New("audit is down")}
	actor := uuid.NewString()

	uc := clinicalapp.NewGetRecordUseCase(repo, &fakeScope{}, audit, accessFor(record.PatientID, actor, false))

	view, err := uc.Execute(context.Background(), clinicalapp.RecordInput{
		TenantID:  string(record.TenantID),
		ActorID:   actor,
		Role:      string(iam.RoleProfessional),
		PatientID: record.PatientID,
	})
	if err == nil {
		t.Fatal("the read succeeded with a broken audit trail: fail closed was not honoured")
	}
	if view.ChiefComplaint != "" {
		t.Fatalf("a refused read returned data anyway: %+v", view)
	}
}

// TestOnlyTheTreatingProfessionalReadsTheRecord pins the refusals. A professional who
// does not treat the patient cannot see the patient at all, so the answer is not-found.
// The owner and the assistant see the patient but never the clinical record, so the
// answer is forbidden. In every refusal nothing is read and nothing is audited.
func TestOnlyTheTreatingProfessionalReadsTheRecord(t *testing.T) {
	record := existingRecord(t)

	cases := map[string]struct {
		role    iam.RoleCode
		actor   string
		wantErr error
	}{
		"another professional does not see the patient": {iam.RoleProfessional, uuid.NewString(), patient.ErrNotFound},
		"the owner sees the patient but not the record": {iam.RoleOwner, uuid.NewString(), care.ErrForbidden},
		"the assistant sees the patient but not the record": {iam.RoleAssistant, uuid.NewString(), care.ErrForbidden},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepository{record: &record}
			audit := &fakeAudit{}
			treating := uuid.NewString()

			uc := clinicalapp.NewGetRecordUseCase(repo, &fakeScope{}, audit, accessFor(record.PatientID, treating, false))
			_, err := uc.Execute(context.Background(), clinicalapp.RecordInput{
				TenantID:  string(record.TenantID),
				ActorID:   tc.actor,
				Role:      string(tc.role),
				PatientID: record.PatientID,
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if len(audit.events) != 0 {
				t.Fatalf("a refused read was audited as if it happened: %+v", audit.events)
			}
		})
	}
}

// TestAGrantReadsButNeverWrites: the granted professional reads the record and is
// audited, and its write is refused before anything is saved.
func TestAGrantReadsButNeverWrites(t *testing.T) {
	record := existingRecord(t)
	granted := uuid.NewString()
	treating := uuid.NewString()
	access := accessFor(record.PatientID, treating, true)

	repo := &fakeRepository{record: &record}
	audit := &fakeAudit{}
	read := clinicalapp.NewGetRecordUseCase(repo, &fakeScope{}, audit, access)
	if _, err := read.Execute(context.Background(), clinicalapp.RecordInput{
		TenantID:  string(record.TenantID),
		ActorID:   granted,
		Role:      string(iam.RoleProfessional),
		PatientID: record.PatientID,
	}); err != nil {
		t.Fatalf("a granted professional could not read: %v", err)
	}

	write := clinicalapp.NewSaveRecordUseCase(repo, &fakeScope{}, audit, fixedClock{now: now}, fixedIDs{}, access)
	_, err := write.Execute(context.Background(), clinicalapp.RecordDraftInput{
		TenantID:       string(record.TenantID),
		ActorID:        granted,
		Role:           string(iam.RoleProfessional),
		PatientID:      record.PatientID,
		ChiefComplaint: "Cambio",
	})
	if !errors.Is(err, care.ErrForbidden) {
		t.Fatalf("a granted professional wrote the record: error = %v, want care.ErrForbidden", err)
	}
	if repo.record.ChiefComplaint != nil && *repo.record.ChiefComplaint == "Cambio" {
		t.Fatal("the refused write reached the repository")
	}
}

func TestNoteAuthorComesFromTheCaller(t *testing.T) {
	repo := &fakeRepository{}
	audit := &fakeAudit{}
	actor := uuid.NewString()
	patientID := uuid.NewString()

	uc := clinicalapp.NewAddNoteUseCase(repo, &fakeScope{}, audit, fixedClock{now: now}, fixedIDs{}, accessFor(patientID, actor, false))

	view, err := uc.Execute(context.Background(), clinicalapp.AddNoteInput{
		TenantID:      uuid.NewString(),
		ActorID:       actor,
		Role:          string(iam.RoleProfessional),
		PatientID:     patientID,
		EncounteredAt: now.Add(-time.Hour),
		Content:       "Sesión de seguimiento",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.AuthorID != actor {
		t.Fatalf("AuthorID = %q, want the caller %q", view.AuthorID, actor)
	}
	if len(repo.notes) != 1 || repo.notes[0].AuthorID != actor {
		t.Fatalf("stored note = %+v, want it signed by the caller", repo.notes)
	}
	if len(audit.events) != 1 || audit.events[0].Action != "clinical.write" {
		t.Fatalf("audit events = %+v, want one clinical.write", audit.events)
	}
}

func TestNotePaginationIsValidatedHere(t *testing.T) {
	uc := clinicalapp.NewListNotesUseCase(&fakeRepository{}, &fakeScope{}, &fakeAudit{}, accessFor(uuid.NewString(), uuid.NewString(), false))

	for name, in := range map[string]clinicalapp.ListNotesInput{
		"negative limit":  {Limit: -1},
		"huge limit":      {Limit: 500},
		"negative offset": {Limit: 10, Offset: -5},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := uc.Execute(context.Background(), in)

			var validation *clinical.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want a ValidationError: a raw SQLSTATE from the driver is what L10 recorded", err)
			}
		})
	}
}

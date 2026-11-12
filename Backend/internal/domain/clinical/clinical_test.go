package clinical_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var now = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

func newNote(t *testing.T, draft clinical.NoteDraft) clinical.Note {
	t.Helper()

	note, err := clinical.NewNote(
		clinical.NoteID(uuid.NewString()),
		tenant.ID(uuid.NewString()),
		uuid.NewString(),
		draft,
		now,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return note
}

func validDraft() clinical.NoteDraft {
	return clinical.NoteDraft{
		PatientID:     uuid.NewString(),
		EncounteredAt: now.Add(-24 * time.Hour),
		Content:       "  La paciente relata mejoría en el sueño.  ",
	}
}

func TestNewNote(t *testing.T) {
	note := newNote(t, validDraft())

	if note.Content != "La paciente relata mejoría en el sueño." {
		t.Errorf("Content = %q, want it trimmed", note.Content)
	}
	if !note.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", note.CreatedAt, now)
	}
	// The encounter happened yesterday even though the row was written now: that
	// difference is the whole reason the column exists.
	if !note.EncounteredAt.Before(note.CreatedAt) {
		t.Errorf("EncounteredAt = %v, want it before CreatedAt %v", note.EncounteredAt, note.CreatedAt)
	}
}

func TestNewNoteValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*clinical.NoteDraft)
		field  string
	}{
		"blank content":     {func(d *clinical.NoteDraft) { d.Content = "   " }, "content"},
		"empty content":     {func(d *clinical.NoteDraft) { d.Content = "" }, "content"},
		"missing patient":   {func(d *clinical.NoteDraft) { d.PatientID = " " }, "patient_id"},
		"missing encounter": {func(d *clinical.NoteDraft) { d.EncounteredAt = time.Time{} }, "encountered_at"},
		// The database cannot check this one: now() is not immutable.
		"future encounter": {func(d *clinical.NoteDraft) { d.EncounteredAt = now.Add(time.Hour) }, "encountered_at"},
	} {
		t.Run(name, func(t *testing.T) {
			draft := validDraft()
			tc.mutate(&draft)

			_, err := clinical.NewNote(
				clinical.NoteID(uuid.NewString()),
				tenant.ID(uuid.NewString()),
				uuid.NewString(),
				draft,
				now,
			)

			var validation *clinical.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want a ValidationError", err)
			}
			if validation.Field != tc.field {
				t.Fatalf("field = %q, want %q", validation.Field, tc.field)
			}
		})
	}
}

// TestNewNoteRequiresAuthor is its own test because it is the invariant that makes a
// note evidence: an anonymous clinical note is not defendable in front of a patient
// or a court, and the author must come from the token.
func TestNewNoteRequiresAuthor(t *testing.T) {
	draft := validDraft()

	for _, author := range []string{"", "   "} {
		_, err := clinical.NewNote(clinical.NoteID(uuid.NewString()), tenant.ID(uuid.NewString()), author, draft, now)

		var validation *clinical.ValidationError
		if !errors.As(err, &validation) || validation.Field != "author_user_id" {
			t.Fatalf("author %q: error = %v, want a ValidationError on author_user_id", author, err)
		}
	}
}

func TestNewNoteRejectsEmptyIdentity(t *testing.T) {
	if _, err := clinical.NewNote("", tenant.ID(uuid.NewString()), uuid.NewString(), validDraft(), now); err == nil {
		t.Fatal("an empty note id must be rejected")
	}
	if _, err := clinical.NewNote(clinical.NoteID(uuid.NewString()), "", uuid.NewString(), validDraft(), now); err == nil {
		t.Fatal("an empty tenant id must be rejected")
	}
}

func TestNewRecord(t *testing.T) {
	empty := "   "
	draft := clinical.RecordDraft{
		ChiefComplaint: stringPtr("  Dificultad para dormir  "),
		Background:     &empty,
		Objectives:     nil,
	}

	record, err := clinical.NewRecord(
		clinical.RecordID(uuid.NewString()),
		tenant.ID(uuid.NewString()),
		uuid.NewString(),
		draft,
		now,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if record.ChiefComplaint == nil || *record.ChiefComplaint != "Dificultad para dormir" {
		t.Errorf("ChiefComplaint = %v, want it trimmed", record.ChiefComplaint)
	}
	// A blank frame field collapses to nil: "cleared" and "never set" are one state.
	if record.Background != nil {
		t.Errorf("Background = %q, want nil", *record.Background)
	}
	if record.Objectives != nil {
		t.Errorf("Objectives = %q, want nil", *record.Objectives)
	}
}

func TestNewRecordRequiresPatient(t *testing.T) {
	_, err := clinical.NewRecord(clinical.RecordID(uuid.NewString()), tenant.ID(uuid.NewString()), "  ", clinical.RecordDraft{}, now)

	var validation *clinical.ValidationError
	if !errors.As(err, &validation) || validation.Field != "patient_id" {
		t.Fatalf("error = %v, want a ValidationError on patient_id", err)
	}
}

func TestRecordRevise(t *testing.T) {
	record, err := clinical.NewRecord(
		clinical.RecordID(uuid.NewString()),
		tenant.ID(uuid.NewString()),
		uuid.NewString(),
		clinical.RecordDraft{ChiefComplaint: stringPtr("Motivo original")},
		now,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	later := now.Add(48 * time.Hour)
	revised := record.Revise(clinical.RecordDraft{Objectives: stringPtr("Sostener el tratamiento")}, later)

	if revised.ID != record.ID || !revised.CreatedAt.Equal(record.CreatedAt) {
		t.Error("Revise changed the identity or the creation time")
	}
	if revised.ChiefComplaint != nil {
		t.Error("Revise kept a field the draft did not carry")
	}
	if revised.Objectives == nil || *revised.Objectives != "Sostener el tratamiento" {
		t.Errorf("Objectives = %v, want the new value", revised.Objectives)
	}
	if !revised.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", revised.UpdatedAt, later)
	}
}

func TestReconstitute(t *testing.T) {
	tenantID := tenant.ID(uuid.NewString())
	patientID := uuid.NewString()

	note, err := clinical.ReconstituteNote(
		clinical.NoteID(uuid.NewString()), tenantID, patientID, uuid.NewString(),
		now.Add(-time.Hour), "Contenido", now, now,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if note.Content != "Contenido" || note.PatientID != patientID {
		t.Fatalf("note = %+v, want the stored values", note)
	}

	if _, err := clinical.ReconstituteNote("", tenantID, patientID, "", now, "x", now, now); err == nil {
		t.Fatal("ReconstituteNote without an id must fail")
	}
}

func stringPtr(value string) *string { return &value }

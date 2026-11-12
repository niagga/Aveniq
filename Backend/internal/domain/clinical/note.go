package clinical

import (
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// NoteID is the opaque identity of a clinical note.
type NoteID string

// Note is one encounter: what happened that day, written by the professional who was
// there.
//
// It is append-only. There is no Revise method and the schema comment says why: a
// correction is a new note. Editing clinical history in place destroys the record
// that a treatment is supposed to be, and "who changed the note and when" is exactly
// the question nobody can answer after an in-place edit.
type Note struct {
	ID        NoteID
	TenantID  tenant.ID
	PatientID string

	// AuthorID comes from the verified access token, never from the request body. A
	// note signed by whoever the client claims is not a record, it is a rumour.
	AuthorID string

	// EncounteredAt is when the session happened, which is not when the row was
	// written: a professional writes on Sunday about Thursday.
	EncounteredAt time.Time
	Content       string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NoteDraft is what a caller may write.
type NoteDraft struct {
	PatientID     string
	EncounteredAt time.Time
	Content       string
}

// NewNote validates and builds a note.
//
// The future check lives here because the database cannot express it: `now()` is not
// immutable, so a CHECK cannot compare against it. Written in the schema comment as
// well, so the next reader does not try to move it back.
func NewNote(id NoteID, tenantID tenant.ID, authorID string, draft NoteDraft, now time.Time) (Note, error) {
	if id == "" {
		return Note{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Note{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if strings.Trim(draft.PatientID, " ") == "" {
		return Note{}, &ValidationError{Field: "patient_id", Message: "must not be empty"}
	}
	if strings.Trim(authorID, " ") == "" {
		// Without an author the note is anonymous, and an anonymous clinical note is
		// not defendable in front of a patient or a court.
		return Note{}, &ValidationError{Field: "author_user_id", Message: "must not be empty"}
	}
	if strings.Trim(draft.Content, " ") == "" {
		return Note{}, &ValidationError{Field: "content", Message: "must not be blank"}
	}
	if draft.EncounteredAt.IsZero() {
		return Note{}, &ValidationError{Field: "encountered_at", Message: "must not be empty"}
	}
	if draft.EncounteredAt.After(now) {
		return Note{}, &ValidationError{Field: "encountered_at", Message: "must not be in the future"}
	}

	return Note{
		ID:            id,
		TenantID:      tenantID,
		PatientID:     draft.PatientID,
		AuthorID:      authorID,
		EncounteredAt: stored(draft.EncounteredAt),
		Content:       strings.TrimSpace(draft.Content),
		CreatedAt:     stored(now),
		UpdatedAt:     stored(now),
	}, nil
}

// ReconstituteNote rebuilds a note from persisted state.
func ReconstituteNote(
	id NoteID,
	tenantID tenant.ID,
	patientID, authorID string,
	encounteredAt time.Time,
	content string,
	createdAt, updatedAt time.Time,
) (Note, error) {
	if id == "" {
		return Note{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}

	return Note{
		ID:            id,
		TenantID:      tenantID,
		PatientID:     patientID,
		AuthorID:      authorID,
		EncounteredAt: encounteredAt.UTC(),
		Content:       content,
		CreatedAt:     createdAt.UTC(),
		UpdatedAt:     updatedAt.UTC(),
	}, nil
}

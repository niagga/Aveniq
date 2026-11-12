package clinicalapp

import (
	"context"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// PageSize defaults and bounds, mirroring the patient roster.
const (
	DefaultNotePageSize int32 = 20
	MaxNotePageSize     int32 = 100
)

// ListNotesInput asks for one page of a patient's encounters.
type ListNotesInput struct {
	TenantID  string
	ActorID   string
	PatientID string
	Limit     int32
	Offset    int32
}

// AddNoteInput asks to append one encounter.
//
// There is no AuthorID field: the author comes from the verified token, and a body
// field with that name would let a client sign a clinical note as somebody else.
type AddNoteInput struct {
	TenantID      string
	ActorID       string
	PatientID     string
	EncounteredAt time.Time
	Content       string
}

// ListNotesUseCase reads a patient's encounters and records the access.
type ListNotesUseCase struct {
	notes port.ClinicalRepository
	scope port.TenantScope
	audit port.AuditLog
}

// NewListNotesUseCase wires the use case.
func NewListNotesUseCase(notes port.ClinicalRepository, scope port.TenantScope, audit port.AuditLog) *ListNotesUseCase {
	return &ListNotesUseCase{notes: notes, scope: scope, audit: audit}
}

// Execute lists the notes, newest first, and audits the read.
func (uc *ListNotesUseCase) Execute(ctx context.Context, in ListNotesInput) ([]NoteView, error) {
	limit, offset, err := pageOf(in.Limit, in.Offset)
	if err != nil {
		return nil, err
	}

	who := caller{tenantID: in.TenantID, actorID: in.ActorID}

	var views []NoteView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		notes, err := uc.notes.NotesByPatient(ctx, in.PatientID, limit, offset)
		if err != nil {
			return err
		}
		if err := auditRead(ctx, uc.audit, who, in.PatientID, "clinical_note"); err != nil {
			return err
		}

		views = make([]NoteView, 0, len(notes))
		for _, note := range notes {
			views = append(views, toNoteView(note))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

// AddNoteUseCase appends one encounter.
type AddNoteUseCase struct {
	notes port.ClinicalRepository
	scope port.TenantScope
	audit port.AuditLog
	clock port.Clock
	ids   port.IDGenerator
}

// NewAddNoteUseCase wires the use case.
func NewAddNoteUseCase(
	notes port.ClinicalRepository,
	scope port.TenantScope,
	audit port.AuditLog,
	clock port.Clock,
	ids port.IDGenerator,
) *AddNoteUseCase {
	return &AddNoteUseCase{notes: notes, scope: scope, audit: audit, clock: clock, ids: ids}
}

// Execute validates and appends the note.
func (uc *AddNoteUseCase) Execute(ctx context.Context, in AddNoteInput) (NoteView, error) {
	who := caller{tenantID: in.TenantID, actorID: in.ActorID}

	// The author is the caller's identity. This is the one line that makes a clinical
	// note evidence rather than a claim.
	note, err := clinical.NewNote(
		clinical.NoteID(uc.ids.NewID()),
		tenant.ID(in.TenantID),
		in.ActorID,
		clinical.NoteDraft{
			PatientID:     in.PatientID,
			EncounteredAt: in.EncounteredAt,
			Content:       in.Content,
		},
		uc.clock.Now(),
	)
	if err != nil {
		return NoteView{}, err
	}

	var view NoteView
	err = uc.scope.WithTenant(ctx, in.TenantID, func(ctx context.Context) error {
		if err := uc.notes.AddNote(ctx, note); err != nil {
			return err
		}
		if err := auditWrite(ctx, uc.audit, who, in.PatientID, "clinical_note"); err != nil {
			return err
		}
		view = toNoteView(note)
		return nil
	})
	if err != nil {
		return NoteView{}, err
	}
	return view, nil
}

// pageOf validates pagination here instead of forwarding it: a negative limit reaching
// the driver surfaces as a raw SQLSTATE, which is what L10 recorded for the first
// listing in this project.
func pageOf(limit, offset int32) (int32, int32, error) {
	if limit == 0 {
		limit = DefaultNotePageSize
	}
	if limit < 1 || limit > MaxNotePageSize {
		return 0, 0, &clinical.ValidationError{Field: "limit", Message: "must be between 1 and 100"}
	}
	if offset < 0 {
		return 0, 0, &clinical.ValidationError{Field: "offset", Message: "must not be negative"}
	}
	return limit, offset, nil
}

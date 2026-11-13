// Package clinicalapp holds the clinical use cases: read the frame of a treatment,
// write it, read the encounters and append one.
//
// It orchestrates the clinical domain through the port contracts and owns two
// decisions that are easy to get wrong elsewhere: every read leaves a trail, and the
// author of a note comes from the verified identity, never from the request body.
package clinicalapp

import (
	"context"
	"time"

	careapp "github.com/niagga/aveniq/backend/internal/application/care"
	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
)

// RecordView is the frame as the API hands it out.
type RecordView struct {
	PatientID      string
	ChiefComplaint string
	Background     string
	Objectives     string
	UpdatedAt      string
}

// NoteView is one encounter as the API hands it out.
type NoteView struct {
	ID            string
	PatientID     string
	AuthorID      string
	EncounteredAt string
	Content       string
}

// toRecordView flattens the aggregate without losing the distinction the screen needs:
// an empty field and a cleared field look the same, which is what the database stores.
func toRecordView(record clinical.Record) RecordView {
	return RecordView{
		PatientID:      record.PatientID,
		ChiefComplaint: deref(record.ChiefComplaint),
		Background:     deref(record.Background),
		Objectives:     deref(record.Objectives),
		UpdatedAt:      record.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toNoteView(note clinical.Note) NoteView {
	return NoteView{
		ID:            string(note.ID),
		PatientID:     note.PatientID,
		AuthorID:      note.AuthorID,
		EncounteredAt: note.EncounteredAt.UTC().Format(time.RFC3339),
		Content:       note.Content,
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// caller is what every clinical use case needs to know about who is asking: the
// practice scopes the transaction, the user becomes the actor of the trail.
type caller struct {
	tenantID string
	actorID  string
}

// checkReadClinical lets a read of the clinical record through only when the caller may
// see the patient and the rule for the clinical record allows it. A patient the caller
// may not see is patient.ErrNotFound; one the caller sees but may not read clinically
// is care.ErrForbidden. Both are checked before any clinical data is loaded.
func checkReadClinical(ctx context.Context, access *careapp.Access, who care.Viewer, patientID string) error {
	resolved, err := access.Resolve(ctx, who, patientID)
	if err != nil {
		return err
	}
	if !care.ReadsClinical(who, resolved.Patient, resolved.Granted) {
		return care.ErrForbidden
	}
	return nil
}

// checkWriteClinical is checkReadClinical for a write: only the treating professional
// writes the clinical record, a granted one reads it.
func checkWriteClinical(ctx context.Context, access *careapp.Access, who care.Viewer, patientID string) error {
	resolved, err := access.Resolve(ctx, who, patientID)
	if err != nil {
		return err
	}
	if !care.WritesClinical(who, resolved.Patient) {
		return care.ErrForbidden
	}
	return nil
}

// auditRead records that clinical data was looked at, inside the caller's transaction.
//
// It is called with the values already read, and its error is returned: a read that
// cannot be audited must not succeed, which is why the write happens in the same
// transaction instead of a background one.
func auditRead(ctx context.Context, log port.AuditLog, who caller, patientID, entity string) error {
	return log.Record(ctx, port.AuditEvent{
		TenantID: who.tenantID,
		ActorID:  who.actorID,
		Action:   "clinical.read",
		Entity:   entity,
		EntityID: patientID,
	})
}

// auditWrite records a change to clinical data.
func auditWrite(ctx context.Context, log port.AuditLog, who caller, patientID, entity string) error {
	return log.Record(ctx, port.AuditEvent{
		TenantID: who.tenantID,
		ActorID:  who.actorID,
		Action:   "clinical.write",
		Entity:   entity,
		EntityID: patientID,
	})
}

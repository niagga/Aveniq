package clinical

import (
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// RecordID is the opaque identity of a clinical record.
type RecordID string

// Record is the stable frame of a treatment: why the person came, what is known and
// what is being aimed at.
//
// It is one per patient (the schema says so), because it is the frame and not a log.
// Its fields are free text on purpose: a structured taxonomy here would be one
// clinical theory baked into a schema, and practices differ.
type Record struct {
	ID        RecordID
	TenantID  tenant.ID
	PatientID string

	ChiefComplaint *string
	Background     *string
	Objectives     *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// RecordDraft is what a caller may write. Empty strings collapse to nil, so
// "cleared" and "never set" are one state instead of two.
type RecordDraft struct {
	ChiefComplaint *string
	Background     *string
	Objectives     *string
}

// NewRecord builds the frame for a patient.
//
// There is no invariant to enforce beyond identity: every clinical field is optional
// because a professional may open the record before the first session and fill it as
// the treatment takes shape. Requiring a complaint would produce a placeholder
// sentence, which is worse than an empty field.
func NewRecord(id RecordID, tenantID tenant.ID, patientID string, draft RecordDraft, now time.Time) (Record, error) {
	if id == "" {
		return Record{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Record{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if strings.Trim(patientID, " ") == "" {
		return Record{}, &ValidationError{Field: "patient_id", Message: "must not be empty"}
	}

	return Record{
		ID:             id,
		TenantID:       tenantID,
		PatientID:      patientID,
		ChiefComplaint: sanitize(draft.ChiefComplaint),
		Background:     sanitize(draft.Background),
		Objectives:     sanitize(draft.Objectives),
		CreatedAt:      stored(now),
		UpdatedAt:      stored(now),
	}, nil
}

// Revise replaces the frame's content, keeping identity and creation time.
func (r Record) Revise(draft RecordDraft, now time.Time) Record {
	r.ChiefComplaint = sanitize(draft.ChiefComplaint)
	r.Background = sanitize(draft.Background)
	r.Objectives = sanitize(draft.Objectives)
	r.UpdatedAt = stored(now)
	return r
}

// ReconstituteRecord rebuilds the frame from persisted state.
func ReconstituteRecord(
	id RecordID,
	tenantID tenant.ID,
	patientID string,
	chiefComplaint, background, objectives *string,
	createdAt, updatedAt time.Time,
) (Record, error) {
	if id == "" {
		return Record{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}

	return Record{
		ID:             id,
		TenantID:       tenantID,
		PatientID:      patientID,
		ChiefComplaint: chiefComplaint,
		Background:     background,
		Objectives:     objectives,
		CreatedAt:      createdAt.UTC(),
		UpdatedAt:      updatedAt.UTC(),
	}, nil
}

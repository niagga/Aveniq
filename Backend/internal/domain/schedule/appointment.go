package schedule

import (
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// AppointmentID is the opaque identity of one slot.
type AppointmentID string

// Status is the lifecycle of a slot.
type Status string

const (
	StatusScheduled Status = "scheduled"
	StatusAttended  Status = "attended"
	StatusCancelled Status = "cancelled"
)

const (
	minDuration = 5 * time.Minute
	maxDuration = 8 * time.Hour
)

// Appointment is one concrete slot. Series expansion produces these; they are
// then edited and cancelled on their own.
type Appointment struct {
	ID                 AppointmentID
	TenantID           tenant.ID
	ProfessionalUserID string
	PatientID          string
	SeriesID           string // empty when the slot is not part of a series
	StartsAt           time.Time
	EndsAt             time.Time
	Status             Status
	Notes              *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// AppointmentDraft is what a caller may write for a one-off slot.
type AppointmentDraft struct {
	ProfessionalUserID string
	PatientID          string
	StartsAt           time.Time
	EndsAt             time.Time
	Notes              *string
}

// NewAppointment builds a scheduled slot.
func NewAppointment(id AppointmentID, tenantID tenant.ID, draft AppointmentDraft, now time.Time) (Appointment, error) {
	if id == "" {
		return Appointment{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Appointment{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if strings.TrimSpace(draft.ProfessionalUserID) == "" {
		return Appointment{}, &ValidationError{Field: "professional_user_id", Message: "must not be empty"}
	}
	if strings.TrimSpace(draft.PatientID) == "" {
		return Appointment{}, &ValidationError{Field: "patient_id", Message: "must not be empty"}
	}

	starts := draft.StartsAt.UTC()
	ends := draft.EndsAt.UTC()
	if starts.IsZero() {
		return Appointment{}, &ValidationError{Field: "starts_at", Message: "must not be empty"}
	}
	if !ends.After(starts) {
		return Appointment{}, &ValidationError{Field: "ends_at", Message: "must be after starts_at"}
	}
	duration := ends.Sub(starts)
	if duration < minDuration {
		return Appointment{}, &ValidationError{Field: "ends_at", Message: "the slot is shorter than 5 minutes"}
	}
	if duration > maxDuration {
		return Appointment{}, &ValidationError{Field: "ends_at", Message: "the slot is longer than 8 hours"}
	}

	return Appointment{
		ID:                 id,
		TenantID:           tenantID,
		ProfessionalUserID: strings.TrimSpace(draft.ProfessionalUserID),
		PatientID:          strings.TrimSpace(draft.PatientID),
		StartsAt:           starts,
		EndsAt:             ends,
		Status:             StatusScheduled,
		Notes:              sanitizeNotes(draft.Notes),
		CreatedAt:          now.UTC(),
		UpdatedAt:          now.UTC(),
	}, nil
}

// Reconstitute rebuilds a slot from persisted state.
func Reconstitute(
	id AppointmentID,
	tenantID tenant.ID,
	professionalUserID, patientID, seriesID string,
	startsAt, endsAt time.Time,
	status Status,
	notes *string,
	createdAt, updatedAt time.Time,
) (Appointment, error) {
	if id == "" {
		return Appointment{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if !status.Valid() {
		return Appointment{}, &ValidationError{Field: "status", Message: "unknown status"}
	}
	return Appointment{
		ID:                 id,
		TenantID:           tenantID,
		ProfessionalUserID: professionalUserID,
		PatientID:          patientID,
		SeriesID:           seriesID,
		StartsAt:           startsAt.UTC(),
		EndsAt:             endsAt.UTC(),
		Status:             status,
		Notes:              notes,
		CreatedAt:          createdAt.UTC(),
		UpdatedAt:          updatedAt.UTC(),
	}, nil
}

// Valid reports whether the status is known.
func (s Status) Valid() bool {
	switch s {
	case StatusScheduled, StatusAttended, StatusCancelled:
		return true
	default:
		return false
	}
}

// Attend marks a scheduled slot as having happened. Attended slots are frozen:
// a later series edit must not rewrite them.
func (a Appointment) Attend(now time.Time) (Appointment, error) {
	if a.Status != StatusScheduled {
		return Appointment{}, ErrInvalidTransition
	}
	a.Status = StatusAttended
	a.UpdatedAt = now.UTC()
	return a, nil
}

// Cancel frees the slot. Cancelled rows drop out of the overlap constraint.
func (a Appointment) Cancel(now time.Time) (Appointment, error) {
	if a.Status != StatusScheduled {
		return Appointment{}, ErrInvalidTransition
	}
	a.Status = StatusCancelled
	a.UpdatedAt = now.UTC()
	return a, nil
}

func sanitizeNotes(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

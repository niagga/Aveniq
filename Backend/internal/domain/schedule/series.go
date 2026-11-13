package schedule

import (
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// SeriesID is the opaque identity of a weekly recurrence.
type SeriesID string

const (
	maxOccurrences = 52
)

// Series is the civil recurrence: a weekday and a local clock time, expanded into
// Appointment rows at write time.
type Series struct {
	ID                 SeriesID
	TenantID           tenant.ID
	ProfessionalUserID string
	PatientID          string
	Weekday            time.Weekday
	LocalStartMinute   int
	DurationMinutes    int
	StartsOn           time.Time // date in the tenant timezone, truncated to day
	UntilOn            time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// SeriesDraft is what a caller may write.
type SeriesDraft struct {
	ProfessionalUserID string
	PatientID          string
	Weekday            int
	LocalStartMinute   int
	DurationMinutes    int
	StartsOn           time.Time
	UntilOn            time.Time
}

// NewSeries builds a weekly series. Expansion happens separately so the use case
// can attach generated ids to each occurrence.
func NewSeries(id SeriesID, tenantID tenant.ID, draft SeriesDraft, now time.Time) (Series, error) {
	if id == "" {
		return Series{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Series{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if strings.TrimSpace(draft.ProfessionalUserID) == "" {
		return Series{}, &ValidationError{Field: "professional_user_id", Message: "must not be empty"}
	}
	if strings.TrimSpace(draft.PatientID) == "" {
		return Series{}, &ValidationError{Field: "patient_id", Message: "must not be empty"}
	}
	if draft.Weekday < 0 || draft.Weekday > 6 {
		return Series{}, &ValidationError{Field: "weekday", Message: "must be between 0 (Sunday) and 6 (Saturday)"}
	}
	if draft.LocalStartMinute < 0 || draft.LocalStartMinute > 1439 {
		return Series{}, &ValidationError{Field: "local_start_minute", Message: "must be between 0 and 1439"}
	}
	if draft.DurationMinutes < 5 || draft.DurationMinutes > 480 {
		return Series{}, &ValidationError{Field: "duration_minutes", Message: "must be between 5 and 480"}
	}
	if draft.StartsOn.IsZero() {
		return Series{}, &ValidationError{Field: "starts_on", Message: "must not be empty"}
	}
	if draft.UntilOn.IsZero() {
		return Series{}, &ValidationError{Field: "until_on", Message: "must not be empty"}
	}

	startsOn := dateOnly(draft.StartsOn)
	untilOn := dateOnly(draft.UntilOn)
	if untilOn.Before(startsOn) {
		return Series{}, &ValidationError{Field: "until_on", Message: "must be on or after starts_on"}
	}

	return Series{
		ID:                 id,
		TenantID:           tenantID,
		ProfessionalUserID: strings.TrimSpace(draft.ProfessionalUserID),
		PatientID:          strings.TrimSpace(draft.PatientID),
		Weekday:            time.Weekday(draft.Weekday),
		LocalStartMinute:   draft.LocalStartMinute,
		DurationMinutes:    draft.DurationMinutes,
		StartsOn:           startsOn,
		UntilOn:            untilOn,
		CreatedAt:          now.UTC(),
		UpdatedAt:          now.UTC(),
	}, nil
}

// ReconstituteSeries rebuilds a series from persisted state.
func ReconstituteSeries(
	id SeriesID,
	tenantID tenant.ID,
	professionalUserID, patientID string,
	weekday, localStartMinute, durationMinutes int,
	startsOn, untilOn, createdAt, updatedAt time.Time,
) (Series, error) {
	return NewSeries(id, tenantID, SeriesDraft{
		ProfessionalUserID: professionalUserID,
		PatientID:          patientID,
		Weekday:            weekday,
		LocalStartMinute:   localStartMinute,
		DurationMinutes:    durationMinutes,
		StartsOn:           startsOn,
		UntilOn:            untilOn,
	}, createdAt)
}

// Expand returns the UTC intervals of this series in loc. It is the whole point
// of storing timezone on the tenant: the same weekday and clock time must not
// drift when DST starts or ends.
func (s Series) Expand(loc *time.Location) ([]Interval, error) {
	if loc == nil {
		return nil, &ValidationError{Field: "timezone", Message: "must not be empty"}
	}

	hour := s.LocalStartMinute / 60
	minute := s.LocalStartMinute % 60
	duration := time.Duration(s.DurationMinutes) * time.Minute

	startDay := time.Date(s.StartsOn.Year(), s.StartsOn.Month(), s.StartsOn.Day(), 0, 0, 0, 0, loc)
	untilDay := time.Date(s.UntilOn.Year(), s.UntilOn.Month(), s.UntilOn.Day(), 0, 0, 0, 0, loc)

	cursor := startDay
	for cursor.Weekday() != s.Weekday {
		cursor = cursor.AddDate(0, 0, 1)
		if cursor.After(untilDay) {
			return nil, &ValidationError{Field: "weekday", Message: "does not occur between starts_on and until_on"}
		}
	}

	intervals := make([]Interval, 0, 8)
	for !cursor.After(untilDay) {
		starts := time.Date(cursor.Year(), cursor.Month(), cursor.Day(), hour, minute, 0, 0, loc)
		intervals = append(intervals, Interval{StartsAt: starts.UTC(), EndsAt: starts.Add(duration).UTC()})
		if len(intervals) > maxOccurrences {
			return nil, &ValidationError{Field: "until_on", Message: "a series cannot emit more than 52 appointments"}
		}
		cursor = cursor.AddDate(0, 0, 7)
	}
	if len(intervals) == 0 {
		return nil, &ValidationError{Field: "weekday", Message: "does not occur between starts_on and until_on"}
	}
	return intervals, nil
}

// Interval is one occurrence before it has an identity.
type Interval struct {
	StartsAt time.Time
	EndsAt   time.Time
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

package clinical

import (
	"strings"
	"time"
)

// sanitize trims an optional free-text field and turns an empty result into nil, so
// "cleared" and "never set" are one state instead of two.
func sanitize(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// stored truncates a timestamp to the resolution Postgres keeps.
//
// The schema stores microseconds, so a nanosecond time comes back different from the
// one written and a comparison across a round trip depends on where it runs.
func stored(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}

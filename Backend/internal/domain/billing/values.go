package billing

import (
	"strings"
	"time"
)

const (
	maxConceptLength = 200
	currencyLength   = 3
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
func stored(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}

func isISOCurrency(code string) bool {
	if len(code) != currencyLength {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func isBlank(value string) bool {
	return strings.TrimSpace(value) == ""
}

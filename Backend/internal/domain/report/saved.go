package report

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Errors of the saved reports.
var (
	// ErrSavedNotFound is a saved report the practice does not hold.
	ErrSavedNotFound = errors.New("report: saved report not found")
	// ErrSavedNameTaken is a name another saved report of the practice already uses.
	ErrSavedNameTaken = errors.New("report: a saved report with that name already exists")
	// ErrTenantScopeMissing is a saved-report operation outside a tenant transaction.
	ErrTenantScopeMissing = errors.New("report: no tenant scope in the context")
)

// MaxNameLength bounds the name of a saved report, in characters.
const MaxNameLength = 80

// SavedID identifies a saved report.
type SavedID string

// Period is the span a saved report asks for. A relative period keeps meaning what it says when
// the report is opened later: a saved "this month" is always the current month.
type Period string

const (
	PeriodThisMonth   Period = "this-month"
	PeriodLastMonth   Period = "last-month"
	PeriodLast3Months Period = "last-3-months"
	PeriodThisYear    Period = "this-year"
	PeriodCustom      Period = "custom"
)

const periodRelativeOnly = "only a custom period carries days"

// SavedDraft is what a person writes when saving a report. From and To are civil days
// (YYYY-MM-DD), both included, and only a custom period has them.
type SavedDraft struct {
	Name    string
	Period  string
	From    string
	To      string
	GroupBy string
	Filter  Filter
}

// SavedReport is the definition of a question the practice asks again and again. It stores no
// figures: opening it asks the billing report, so the answer is today's and is narrowed to the
// reader's own scope.
type SavedReport struct {
	ID        SavedID
	TenantID  string
	Name      string
	Period    Period
	From      string
	To        string
	GroupBy   Dimension
	Filter    Filter
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewSaved validates a draft and builds the saved report.
func NewSaved(id SavedID, tenantID, createdBy string, draft SavedDraft, now time.Time) (SavedReport, error) {
	checked, err := checkDraft(draft)
	if err != nil {
		return SavedReport{}, err
	}
	if createdBy == "" {
		return SavedReport{}, &ValidationError{Field: "created_by", Message: "must not be empty"}
	}
	checked.ID = id
	checked.TenantID = tenantID
	checked.CreatedBy = createdBy
	checked.CreatedAt = now.UTC()
	checked.UpdatedAt = now.UTC()
	return checked, nil
}

// Revise replaces what the report asks. Its identity, its author and its creation time stay.
func (s SavedReport) Revise(draft SavedDraft, now time.Time) (SavedReport, error) {
	checked, err := checkDraft(draft)
	if err != nil {
		return SavedReport{}, err
	}
	checked.ID = s.ID
	checked.TenantID = s.TenantID
	checked.CreatedBy = s.CreatedBy
	checked.CreatedAt = s.CreatedAt
	checked.UpdatedAt = now.UTC()
	if checked.UpdatedAt.Before(s.CreatedAt) {
		checked.UpdatedAt = s.CreatedAt
	}
	return checked, nil
}

// checkDraft validates what is asked and returns it normalized: trimmed name, known period, days
// only where they belong.
func checkDraft(draft SavedDraft) (SavedReport, error) {
	name := strings.TrimSpace(draft.Name)
	if name == "" {
		return SavedReport{}, &ValidationError{Field: "name", Message: "must not be blank"}
	}
	if utf8.RuneCountInString(name) > MaxNameLength {
		return SavedReport{}, &ValidationError{Field: "name", Message: "is too long"}
	}

	period := Period(strings.TrimSpace(draft.Period))
	from, to := strings.TrimSpace(draft.From), strings.TrimSpace(draft.To)
	switch period {
	case PeriodThisMonth, PeriodLastMonth, PeriodLast3Months, PeriodThisYear:
		if from != "" || to != "" {
			return SavedReport{}, &ValidationError{Field: "from", Message: periodRelativeOnly}
		}
	case PeriodCustom:
		if err := checkDays(from, to); err != nil {
			return SavedReport{}, err
		}
	default:
		return SavedReport{}, &ValidationError{Field: "period", Message: "must be this-month, last-month, last-3-months, this-year or custom"}
	}

	groupBy, err := ParseDimension(draft.GroupBy)
	if err != nil {
		return SavedReport{}, err
	}
	filter := draft.Filter
	if err := filter.Validate(); err != nil {
		return SavedReport{}, err
	}

	return SavedReport{Name: name, Period: period, From: from, To: to, GroupBy: groupBy, Filter: filter}, nil
}

// checkDays validates a custom range: two civil days, in order, no longer than a report may be.
func checkDays(from, to string) error {
	first, err := time.Parse(civilDay, from)
	if err != nil {
		return &ValidationError{Field: "from", Message: "must be a day as YYYY-MM-DD"}
	}
	last, err := time.Parse(civilDay, to)
	if err != nil {
		return &ValidationError{Field: "to", Message: "must be a day as YYYY-MM-DD"}
	}
	if last.Before(first) {
		return &ValidationError{Field: "to", Message: "must not be before from"}
	}
	if last.Sub(first) > time.Duration(MaxRangeDays)*24*time.Hour {
		return &ValidationError{Field: "to", Message: "the range is too long"}
	}
	return nil
}

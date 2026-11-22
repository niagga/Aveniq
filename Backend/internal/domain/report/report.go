// Package report is the arithmetic of the billing report: what the practice billed and what it
// attended without billing, by currency and by one cut.
//
// It is pure. The repositories hand it the lines, the application values the sessions with the
// tariff in force, and Build only adds. Two rules hold everywhere in it:
//
//   - Money of different currencies is never added. Every figure is a list with one entry per
//     currency, and there is no conversion.
//   - A session that has no tariff is counted apart, never as zero: the practice must see that
//     it has attended work it cannot value, not a total that looks complete.
package report

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrForbidden is a role that may read no report at all.
var ErrForbidden = errors.New("report: the role may not read reports")

// ValidationError names the request field that was refused, so the interface can say which one.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// MaxRangeDays bounds one report. The sessions are valued one by one, so the range cannot be
// open-ended; five years is more than any practice compares.
const MaxRangeDays = 1830

// Dimension is the cut a report is grouped by.
type Dimension string

const (
	// DimensionNone is no cut: only the totals.
	DimensionNone Dimension = "none"
	// DimensionProfessional cuts by the professional who gave or billed the session.
	DimensionProfessional Dimension = "professional"
	// DimensionCoverage cuts by private or covered by a mutual.
	DimensionCoverage Dimension = "coverage"
	// DimensionInsurer cuts by mutual. Private patients and loose charges have no mutual and
	// fall under the empty key.
	DimensionInsurer Dimension = "insurer"
	// DimensionModality cuts by online or presencial.
	DimensionModality Dimension = "modality"
	// DimensionWeek cuts by calendar week, Monday to Sunday, in the practice's time zone.
	DimensionWeek Dimension = "week"
	// DimensionMonth cuts by calendar month in the practice's time zone.
	DimensionMonth Dimension = "month"
	// DimensionYear cuts by calendar year in the practice's time zone.
	DimensionYear Dimension = "year"
)

// ParseDimension reads a cut. An empty text is no cut.
func ParseDimension(raw string) (Dimension, error) {
	switch dimension := Dimension(strings.TrimSpace(raw)); dimension {
	case "", DimensionNone:
		return DimensionNone, nil
	case DimensionProfessional, DimensionCoverage, DimensionInsurer, DimensionModality,
		DimensionWeek, DimensionMonth, DimensionYear:
		return dimension, nil
	default:
		return "", &ValidationError{Field: "groupBy", Message: "must be none, professional, coverage, insurer, modality, week, month or year"}
	}
}

// The coverage and modality words, the same ones the vouchers and the schedule use.
const (
	CoveragePrivate = "private"
	CoverageInsurer = "insurer"

	ModalityOnline     = "online"
	ModalityPresencial = "presencial"
)

// Filter narrows a report. An empty field does not narrow.
type Filter struct {
	ProfessionalID string
	Coverage       string
	InsurerID      string
	Modality       string
}

// Validate refuses a filter that cannot match anything or cannot be asked of the database.
func (f Filter) Validate() error {
	if f.ProfessionalID != "" {
		if _, err := uuid.Parse(f.ProfessionalID); err != nil {
			return &ValidationError{Field: "professionalId", Message: "must be a UUID"}
		}
	}
	if f.InsurerID != "" {
		if _, err := uuid.Parse(f.InsurerID); err != nil {
			return &ValidationError{Field: "insurerId", Message: "must be a UUID"}
		}
	}
	switch f.Coverage {
	case "", CoveragePrivate, CoverageInsurer:
	default:
		return &ValidationError{Field: "coverage", Message: "must be private or insurer"}
	}
	if f.InsurerID != "" && f.Coverage == CoveragePrivate {
		return &ValidationError{Field: "insurerId", Message: "a private patient has no mutual"}
	}
	switch f.Modality {
	case "", ModalityOnline, ModalityPresencial:
	default:
		return &ValidationError{Field: "modality", Message: "must be online or presencial"}
	}
	return nil
}

// Window is the range of a report as instants: From is included, To is not.
type Window struct {
	From time.Time
	To   time.Time
}

const civilDay = "2006-01-02"

// ParseWindow turns two civil days of the practice, both included, into the instants that bound
// them in the practice's time zone. The range is read in that zone, not in UTC: a session at
// 23:30 in Buenos Aires on the last day belongs to that day even though it is the next one in UTC.
func ParseWindow(from, to string, location *time.Location) (Window, error) {
	first, err := time.ParseInLocation(civilDay, from, location)
	if err != nil {
		return Window{}, &ValidationError{Field: "from", Message: "must be a day as YYYY-MM-DD"}
	}
	last, err := time.ParseInLocation(civilDay, to, location)
	if err != nil {
		return Window{}, &ValidationError{Field: "to", Message: "must be a day as YYYY-MM-DD"}
	}
	if last.Before(first) {
		return Window{}, &ValidationError{Field: "to", Message: "must not be before from"}
	}
	end := time.Date(last.Year(), last.Month(), last.Day()+1, 0, 0, 0, 0, location)
	startDay := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, location)
	if end.Sub(startDay) > time.Duration(MaxRangeDays+1)*24*time.Hour {
		return Window{}, &ValidationError{Field: "to", Message: "the range is too long"}
	}
	return Window{From: startDay.UTC(), To: end.UTC()}, nil
}

// Line is what a report needs to know about one billed voucher or one attended session: when it
// counts, in which currency, and the cuts it can be grouped by. Coverage, insurer, modality and
// professional may be empty for an old voucher or a loose charge, which carry none of them.
type Line struct {
	At               time.Time
	Currency         string
	Coverage         string
	InsurerID        string
	InsurerName      string
	Modality         string
	ProfessionalID   string
	ProfessionalName string
}

// BilledLine is an issued voucher: the amount it billed.
type BilledLine struct {
	Line
	AmountMinor int64
}

// PendingLine is an attended session with no issued voucher. It is Valued when a tariff was in
// force on its day, and then carries the full amount the practice would bill for it.
type PendingLine struct {
	Line
	Valued      bool
	AmountMinor int64
}

// Money is the sum of one currency and how many lines made it.
type Money struct {
	Currency    string
	Count       int
	AmountMinor int64
}

// Figures are the numbers of one row or of the whole report.
type Figures struct {
	// Billed is what the issued vouchers billed, by currency.
	Billed []Money
	// Pending is what the attended sessions without a voucher are worth, by currency. Only the
	// valued sessions are in it.
	Pending []Money
	// Unvalued counts the sessions without a voucher that no tariff values.
	Unvalued int
}

// Row is one group of a report.
type Row struct {
	Key     string
	Label   string
	Figures Figures
}

// Report is the totals and, when a cut was asked for, one row per group.
type Report struct {
	Totals Figures
	Rows   []Row
}

// Build adds the lines. The practice's location decides which week, month or year a line falls in.
func Build(dimension Dimension, location *time.Location, billed []BilledLine, pending []PendingLine) Report {
	total := newTally()
	groups := map[string]*group{}

	reach := func(line Line) *tally {
		if dimension == DimensionNone {
			return nil
		}
		key, label := cut(dimension, location, line)
		found, ok := groups[key]
		if !ok {
			found = &group{label: label, tally: newTally()}
			groups[key] = found
		}
		if found.label == "" {
			found.label = label
		}
		return found.tally
	}

	for _, line := range billed {
		total.bill(line.Currency, line.AmountMinor)
		if rows := reach(line.Line); rows != nil {
			rows.bill(line.Currency, line.AmountMinor)
		}
	}
	for _, line := range pending {
		total.owe(line)
		if rows := reach(line.Line); rows != nil {
			rows.owe(line)
		}
	}

	report := Report{Totals: total.figures(), Rows: []Row{}}
	for key, found := range groups {
		report.Rows = append(report.Rows, Row{Key: key, Label: found.label, Figures: found.tally.figures()})
	}
	sortRows(dimension, report.Rows)
	return report
}

type group struct {
	label string
	tally *tally
}

// tally accumulates one set of figures.
type tally struct {
	billed   map[string]*Money
	pending  map[string]*Money
	unvalued int
}

func newTally() *tally {
	return &tally{billed: map[string]*Money{}, pending: map[string]*Money{}}
}

func (t *tally) bill(currency string, amount int64) { add(t.billed, currency, amount) }

func (t *tally) owe(line PendingLine) {
	if !line.Valued {
		t.unvalued++
		return
	}
	add(t.pending, line.Currency, line.AmountMinor)
}

func add(into map[string]*Money, currency string, amount int64) {
	money, ok := into[currency]
	if !ok {
		money = &Money{Currency: currency}
		into[currency] = money
	}
	money.Count++
	money.AmountMinor += amount
}

func (t *tally) figures() Figures {
	return Figures{Billed: sorted(t.billed), Pending: sorted(t.pending), Unvalued: t.unvalued}
}

func sorted(from map[string]*Money) []Money {
	out := make([]Money, 0, len(from))
	for _, money := range from {
		out = append(out, *money)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}

// cut returns the key and the label of the group a line falls in. A line without the cut has the
// empty key: it is shown, never dropped, so the rows always add up to the totals.
func cut(dimension Dimension, location *time.Location, line Line) (key, label string) {
	switch dimension {
	case DimensionProfessional:
		return line.ProfessionalID, line.ProfessionalName
	case DimensionCoverage:
		return line.Coverage, line.Coverage
	case DimensionInsurer:
		return line.InsurerID, line.InsurerName
	case DimensionModality:
		return line.Modality, line.Modality
	case DimensionWeek:
		day := line.At.In(location)
		offset := (int(day.Weekday()) + 6) % 7 // Monday is 0
		monday := time.Date(day.Year(), day.Month(), day.Day()-offset, 0, 0, 0, 0, location)
		key = monday.Format(civilDay)
		return key, key
	case DimensionMonth:
		key = line.At.In(location).Format("2006-01")
		return key, key
	case DimensionYear:
		key = line.At.In(location).Format("2006")
		return key, key
	default:
		return "", ""
	}
}

// sortRows orders periods from the oldest, and the other cuts by name with the group that has no
// value last.
func sortRows(dimension Dimension, rows []Row) {
	byTime := dimension == DimensionWeek || dimension == DimensionMonth || dimension == DimensionYear
	sort.Slice(rows, func(i, j int) bool {
		if byTime {
			return rows[i].Key < rows[j].Key
		}
		if (rows[i].Key == "") != (rows[j].Key == "") {
			return rows[j].Key == ""
		}
		left, right := strings.ToLower(rows[i].Label), strings.ToLower(rows[j].Label)
		if left != right {
			return left < right
		}
		return rows[i].Key < rows[j].Key
	})
}

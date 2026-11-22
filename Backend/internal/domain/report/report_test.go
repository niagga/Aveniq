package report_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/report"
)

func buenosAires(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("cannot load the time zone: %v", err)
	}
	return location
}

func moneyOf(figures []report.Money, currency string) (report.Money, bool) {
	for _, money := range figures {
		if money.Currency == currency {
			return money, true
		}
	}
	return report.Money{}, false
}

func billedLine(at time.Time, currency string, amount int64, line report.Line) report.BilledLine {
	line.At, line.Currency = at, currency
	return report.BilledLine{Line: line, AmountMinor: amount}
}

func pendingLine(at time.Time, currency string, amount int64, valued bool, line report.Line) report.PendingLine {
	line.At, line.Currency = at, currency
	return report.PendingLine{Line: line, Valued: valued, AmountMinor: amount}
}

func TestParseDimension(t *testing.T) {
	for raw, want := range map[string]report.Dimension{
		"": report.DimensionNone, "none": report.DimensionNone, "professional": report.DimensionProfessional,
		"coverage": report.DimensionCoverage, "insurer": report.DimensionInsurer, "modality": report.DimensionModality,
		"week": report.DimensionWeek, "month": report.DimensionMonth, "year": report.DimensionYear,
	} {
		got, err := report.ParseDimension(raw)
		if err != nil || got != want {
			t.Errorf("ParseDimension(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	var invalid *report.ValidationError
	if _, err := report.ParseDimension("day"); !errors.As(err, &invalid) || invalid.Field != "groupBy" {
		t.Errorf("an unknown cut must be refused on groupBy, got %v", err)
	}
}

func TestFilterValidate(t *testing.T) {
	const id = "7f1e1b6e-8a6c-4a43-9c53-3f1f2a4b5c6d"
	cases := []struct {
		name   string
		filter report.Filter
		field  string
	}{
		{"empty", report.Filter{}, ""},
		{"full", report.Filter{ProfessionalID: id, Coverage: "insurer", InsurerID: id, Modality: "online"}, ""},
		{"professional not a uuid", report.Filter{ProfessionalID: "x"}, "professionalId"},
		{"insurer not a uuid", report.Filter{InsurerID: "x"}, "insurerId"},
		{"coverage unknown", report.Filter{Coverage: "other"}, "coverage"},
		{"private with a mutual", report.Filter{Coverage: "private", InsurerID: id}, "insurerId"},
		{"modality unknown", report.Filter{Modality: "phone"}, "modality"},
	}
	for _, tc := range cases {
		err := tc.filter.Validate()
		if tc.field == "" {
			if err != nil {
				t.Errorf("%s: unexpected %v", tc.name, err)
			}
			continue
		}
		var invalid *report.ValidationError
		if !errors.As(err, &invalid) || invalid.Field != tc.field {
			t.Errorf("%s: got %v, want a refusal on %s", tc.name, err, tc.field)
		}
	}
}

func TestTheWindowIsReadInThePracticeZoneAndIncludesTheLastDay(t *testing.T) {
	location := buenosAires(t)
	window, err := report.ParseWindow("2026-10-01", "2026-10-31", location)
	if err != nil {
		t.Fatalf("ParseWindow: %v", err)
	}
	// Midnight in Buenos Aires is 03:00 UTC.
	if want := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC); !window.From.Equal(want) {
		t.Errorf("From = %v, want %v", window.From, want)
	}
	if want := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC); !window.To.Equal(want) {
		t.Errorf("To = %v, want %v (the day after the last one, excluded)", window.To, want)
	}

	if _, err := report.ParseWindow("2026-10-05", "2026-10-05", location); err != nil {
		t.Errorf("a single day must be a valid range: %v", err)
	}
}

func TestTheWindowRefusesWhatItCannotRead(t *testing.T) {
	location := buenosAires(t)
	cases := []struct{ name, from, to, field string }{
		{"from malformed", "01/10/2026", "2026-10-31", "from"},
		{"to malformed", "2026-10-01", "mañana", "to"},
		{"reversed", "2026-10-31", "2026-10-01", "to"},
		{"too long", "2020-01-01", "2026-12-31", "to"},
	}
	for _, tc := range cases {
		_, err := report.ParseWindow(tc.from, tc.to, location)
		var invalid *report.ValidationError
		if !errors.As(err, &invalid) || invalid.Field != tc.field {
			t.Errorf("%s: got %v, want a refusal on %s", tc.name, err, tc.field)
		}
	}
}

func TestMoneyOfDifferentCurrenciesIsNeverAdded(t *testing.T) {
	at := time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)
	built := report.Build(report.DimensionNone, buenosAires(t),
		[]report.BilledLine{
			billedLine(at, "ARS", 1000000, report.Line{}),
			billedLine(at, "ARS", 500000, report.Line{}),
			billedLine(at, "USD", 8000, report.Line{}),
		},
		[]report.PendingLine{
			pendingLine(at, "ARS", 900000, true, report.Line{}),
			pendingLine(at, "USD", 7000, true, report.Line{}),
		})

	ars, _ := moneyOf(built.Totals.Billed, "ARS")
	if ars.AmountMinor != 1500000 || ars.Count != 2 {
		t.Errorf("billed ARS = %+v, want 1500000 in 2 vouchers", ars)
	}
	usd, _ := moneyOf(built.Totals.Billed, "USD")
	if usd.AmountMinor != 8000 || usd.Count != 1 {
		t.Errorf("billed USD = %+v, want 8000 in 1 voucher", usd)
	}
	if len(built.Totals.Pending) != 2 {
		t.Errorf("pending = %+v, want one entry per currency", built.Totals.Pending)
	}
	if built.Totals.Billed[0].Currency != "ARS" || built.Totals.Billed[1].Currency != "USD" {
		t.Errorf("currencies must come in a stable order: %+v", built.Totals.Billed)
	}
	if len(built.Rows) != 0 {
		t.Errorf("no cut means no rows, got %d", len(built.Rows))
	}
}

func TestASessionWithoutTariffIsCountedApartNotAsZero(t *testing.T) {
	at := time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)
	built := report.Build(report.DimensionNone, buenosAires(t), nil,
		[]report.PendingLine{
			pendingLine(at, "ARS", 900000, true, report.Line{}),
			pendingLine(at, "ARS", 0, false, report.Line{}),
			pendingLine(at, "ARS", 0, false, report.Line{}),
		})
	if built.Totals.Unvalued != 2 {
		t.Errorf("unvalued = %d, want 2", built.Totals.Unvalued)
	}
	ars, _ := moneyOf(built.Totals.Pending, "ARS")
	if ars.Count != 1 || ars.AmountMinor != 900000 {
		t.Errorf("pending ARS = %+v, want only the valued session", ars)
	}
}

func TestTheRowsOfAProfessionalCutAddUpToTheTotals(t *testing.T) {
	at := time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)
	ana := report.Line{ProfessionalID: "p-ana", ProfessionalName: "Ana"}
	bruno := report.Line{ProfessionalID: "p-bruno", ProfessionalName: "bruno"}
	loose := report.Line{} // a loose charge names no professional
	built := report.Build(report.DimensionProfessional, buenosAires(t),
		[]report.BilledLine{
			billedLine(at, "ARS", 100, bruno),
			billedLine(at, "ARS", 200, ana),
			billedLine(at, "ARS", 400, loose),
		},
		[]report.PendingLine{pendingLine(at, "ARS", 50, true, ana)})

	if len(built.Rows) != 3 {
		t.Fatalf("rows = %+v, want 3", built.Rows)
	}
	// By name, case-insensitive, the group without a professional last.
	if built.Rows[0].Label != "Ana" || built.Rows[1].Label != "bruno" || built.Rows[2].Key != "" {
		t.Errorf("rows out of order: %+v", built.Rows)
	}
	var sum int64
	for _, row := range built.Rows {
		money, _ := moneyOf(row.Figures.Billed, "ARS")
		sum += money.AmountMinor
	}
	total, _ := moneyOf(built.Totals.Billed, "ARS")
	if sum != total.AmountMinor || total.AmountMinor != 700 {
		t.Errorf("rows add %d, total %d, want both 700", sum, total.AmountMinor)
	}
	anaPending, _ := moneyOf(built.Rows[0].Figures.Pending, "ARS")
	if anaPending.AmountMinor != 50 {
		t.Errorf("Ana pending = %+v, want 50", anaPending)
	}
}

func TestAPeriodCutFollowsThePracticeCalendar(t *testing.T) {
	location := buenosAires(t)
	// Sunday 25 October 2026 at 23:30 in Buenos Aires is Monday 26 at 02:30 UTC: still the
	// week of Monday 19 and still October for the practice.
	lateSunday := time.Date(2026, 10, 26, 2, 30, 0, 0, time.UTC)
	nextMonday := time.Date(2026, 10, 26, 3, 30, 0, 0, time.UTC)
	newYear := time.Date(2027, 1, 1, 2, 0, 0, 0, time.UTC) // 31 December 23:00 in Buenos Aires

	billed := []report.BilledLine{
		billedLine(lateSunday, "ARS", 1, report.Line{}),
		billedLine(nextMonday, "ARS", 10, report.Line{}),
		billedLine(newYear, "ARS", 100, report.Line{}),
	}

	weeks := report.Build(report.DimensionWeek, location, billed, nil)
	if len(weeks.Rows) != 3 || weeks.Rows[0].Key != "2026-10-19" || weeks.Rows[1].Key != "2026-10-26" || weeks.Rows[2].Key != "2026-12-28" {
		t.Errorf("weeks = %+v, want the Mondays 2026-10-19, 2026-10-26 and 2026-12-28", weeks.Rows)
	}

	months := report.Build(report.DimensionMonth, location, billed, nil)
	if len(months.Rows) != 2 || months.Rows[0].Key != "2026-10" || months.Rows[1].Key != "2026-12" {
		t.Errorf("months = %+v, want 2026-10 and 2026-12", months.Rows)
	}
	if october, _ := moneyOf(months.Rows[0].Figures.Billed, "ARS"); october.AmountMinor != 11 {
		t.Errorf("October = %+v, want 11", october)
	}

	years := report.Build(report.DimensionYear, location, billed, nil)
	if len(years.Rows) != 1 || years.Rows[0].Key != "2026" {
		t.Errorf("years = %+v, want only 2026 (31 December at night is still 2026 in the practice)", years.Rows)
	}
}

func TestTheInsurerCutKeepsPrivatePatientsUnderTheEmptyKey(t *testing.T) {
	at := time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)
	osde := report.Line{Coverage: "insurer", InsurerID: "i-osde", InsurerName: "OSDE"}
	private := report.Line{Coverage: "private"}
	built := report.Build(report.DimensionInsurer, buenosAires(t),
		[]report.BilledLine{billedLine(at, "ARS", 3, osde), billedLine(at, "ARS", 4, private)}, nil)
	if len(built.Rows) != 2 || built.Rows[0].Label != "OSDE" || built.Rows[1].Key != "" {
		t.Errorf("rows = %+v, want OSDE then the group without a mutual", built.Rows)
	}

	coverage := report.Build(report.DimensionCoverage, buenosAires(t),
		[]report.BilledLine{billedLine(at, "ARS", 3, osde), billedLine(at, "ARS", 4, private)}, nil)
	if len(coverage.Rows) != 2 || coverage.Rows[0].Key != "insurer" || coverage.Rows[1].Key != "private" {
		t.Errorf("coverage rows = %+v", coverage.Rows)
	}
}

package report_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/report"
)

var savedNow = time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)

func validDraft() report.SavedDraft {
	return report.SavedDraft{Name: "  Mes por mutual ", Period: "this-month", GroupBy: "insurer"}
}

func TestASavedReportKeepsTheDefinitionAndTrimsTheName(t *testing.T) {
	saved, err := report.NewSaved("id-1", "tenant-1", "user-1", report.SavedDraft{
		Name: "  Mes por mutual ", Period: "this-month", GroupBy: "insurer",
		Filter: report.Filter{Coverage: "insurer", Modality: "presencial"},
	}, savedNow)
	if err != nil {
		t.Fatalf("NewSaved: %v", err)
	}
	if saved.Name != "Mes por mutual" || saved.Period != report.PeriodThisMonth || saved.GroupBy != report.DimensionInsurer {
		t.Errorf("saved = %+v", saved)
	}
	if saved.Filter.Coverage != "insurer" || saved.Filter.Modality != "presencial" {
		t.Errorf("filter = %+v, want it kept", saved.Filter)
	}
	if saved.CreatedBy != "user-1" || !saved.CreatedAt.Equal(savedNow) || !saved.UpdatedAt.Equal(savedNow) {
		t.Errorf("author and times = %+v", saved)
	}
}

func TestACutLeftEmptyMeansNoCut(t *testing.T) {
	saved, err := report.NewSaved("id-1", "tenant-1", "user-1", report.SavedDraft{Name: "Totales", Period: "this-year"}, savedNow)
	if err != nil || saved.GroupBy != report.DimensionNone {
		t.Errorf("saved = %+v, err = %v; want no cut", saved, err)
	}
}

func TestACustomPeriodCarriesTwoDaysAndOnlyACustomOneDoes(t *testing.T) {
	custom := report.SavedDraft{Name: "Octubre", Period: "custom", From: "2026-10-01", To: "2026-10-31"}
	saved, err := report.NewSaved("id-1", "tenant-1", "user-1", custom, savedNow)
	if err != nil || saved.From != "2026-10-01" || saved.To != "2026-10-31" {
		t.Fatalf("custom = %+v, err = %v", saved, err)
	}

	relative := report.SavedDraft{Name: "Mes", Period: "this-month", From: "2026-10-01", To: "2026-10-31"}
	var invalid *report.ValidationError
	if _, err := report.NewSaved("id-1", "tenant-1", "user-1", relative, savedNow); !errors.As(err, &invalid) || invalid.Field != "from" {
		t.Errorf("a relative period with days: %v, want a refusal on from", err)
	}
}

func TestASavedReportRefusesWhatItCannotAsk(t *testing.T) {
	cases := []struct {
		name  string
		draft func(*report.SavedDraft)
		field string
	}{
		{"blank name", func(d *report.SavedDraft) { d.Name = "   " }, "name"},
		{"long name", func(d *report.SavedDraft) { d.Name = strings.Repeat("a", report.MaxNameLength+1) }, "name"},
		{"unknown period", func(d *report.SavedDraft) { d.Period = "last-week" }, "period"},
		{"custom without days", func(d *report.SavedDraft) { d.Period = "custom" }, "from"},
		{"custom with a malformed day", func(d *report.SavedDraft) { d.Period, d.From, d.To = "custom", "2026-10-01", "x" }, "to"},
		{"custom reversed", func(d *report.SavedDraft) { d.Period, d.From, d.To = "custom", "2026-10-31", "2026-10-01" }, "to"},
		{"custom too long", func(d *report.SavedDraft) { d.Period, d.From, d.To = "custom", "2018-01-01", "2026-12-31" }, "to"},
		{"unknown cut", func(d *report.SavedDraft) { d.GroupBy = "day" }, "groupBy"},
		{"unknown coverage", func(d *report.SavedDraft) { d.Filter.Coverage = "other" }, "coverage"},
		{"professional not a uuid", func(d *report.SavedDraft) { d.Filter.ProfessionalID = "x" }, "professionalId"},
	}
	for _, tc := range cases {
		draft := validDraft()
		tc.draft(&draft)
		_, err := report.NewSaved("id-1", "tenant-1", "user-1", draft, savedNow)
		var invalid *report.ValidationError
		if !errors.As(err, &invalid) || invalid.Field != tc.field {
			t.Errorf("%s: got %v, want a refusal on %s", tc.name, err, tc.field)
		}
	}
}

func TestAnAuthorIsRequired(t *testing.T) {
	_, err := report.NewSaved("id-1", "tenant-1", "", validDraft(), savedNow)
	var invalid *report.ValidationError
	if !errors.As(err, &invalid) || invalid.Field != "created_by" {
		t.Errorf("no author: %v, want a refusal on created_by", err)
	}
}

func TestRevisingReplacesWhatIsAskedAndKeepsWhoAndWhen(t *testing.T) {
	saved, err := report.NewSaved("id-1", "tenant-1", "user-1", validDraft(), savedNow)
	if err != nil {
		t.Fatalf("NewSaved: %v", err)
	}
	later := savedNow.Add(48 * time.Hour)
	revised, err := saved.Revise(report.SavedDraft{Name: "Año por profesional", Period: "this-year", GroupBy: "professional"}, later)
	if err != nil {
		t.Fatalf("Revise: %v", err)
	}
	if revised.Name != "Año por profesional" || revised.Period != report.PeriodThisYear || revised.GroupBy != report.DimensionProfessional {
		t.Errorf("revised = %+v", revised)
	}
	if revised.ID != saved.ID || revised.TenantID != saved.TenantID || revised.CreatedBy != "user-1" || !revised.CreatedAt.Equal(savedNow) {
		t.Errorf("identity, author and creation must stay: %+v", revised)
	}
	if !revised.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", revised.UpdatedAt, later)
	}
	if revised.Filter.Coverage != "" {
		t.Errorf("a revision replaces the filters, it does not merge them: %+v", revised.Filter)
	}

	// A clock that went backwards never leaves the report updated before it was created.
	early, err := saved.Revise(validDraft(), savedNow.Add(-time.Hour))
	if err != nil || early.UpdatedAt.Before(early.CreatedAt) {
		t.Errorf("UpdatedAt %v before CreatedAt %v (err %v)", early.UpdatedAt, early.CreatedAt, err)
	}
}

package pricing_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/schedule"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func insurerID() *string {
	id := "11111111-1111-1111-1111-111111111111"
	return &id
}

func privateDraft() pricing.Draft {
	return pricing.Draft{
		ProfessionalUserID: "22222222-2222-2222-2222-222222222222",
		Coverage:           patient.CoveragePrivate,
		Modality:           schedule.ModalityPresencial,
		Currency:           patient.CurrencyARS,
		ValidFrom:          time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		PatientMinor:       1_500_000,
	}
}

func insurerDraft() pricing.Draft {
	return pricing.Draft{
		ProfessionalUserID: "22222222-2222-2222-2222-222222222222",
		Coverage:           patient.CoverageInsurer,
		InsurerID:          insurerID(),
		Modality:           schedule.ModalityOnline,
		Currency:           patient.CurrencyARS,
		ValidFrom:          time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		InsurerMinor:       1_000_000,
		PatientMinor:       200_000,
	}
}

func validationField(t *testing.T, err error) string {
	t.Helper()
	var validation *pricing.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want a ValidationError", err)
	}
	return validation.Field
}

func TestNewParticularPriceIsTheWholeFeePaidByThePatient(t *testing.T) {
	subject, err := pricing.New("tar-1", tenant.ID("tenant-1"), privateDraft(), now)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if subject.InsurerMinor != 0 || subject.PatientMinor != 1_500_000 {
		t.Fatalf("split = %d/%d, want 0/1500000", subject.InsurerMinor, subject.PatientMinor)
	}
	if subject.TotalMinor() != 1_500_000 {
		t.Fatalf("total = %d, want 1500000", subject.TotalMinor())
	}
	if subject.InsurerID != nil {
		t.Fatalf("a particular price names no mutual, got %q", *subject.InsurerID)
	}
}

func TestNewMutualPriceSplitsTheSession(t *testing.T) {
	subject, err := pricing.New("tar-1", tenant.ID("tenant-1"), insurerDraft(), now)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if subject.TotalMinor() != 1_200_000 {
		t.Fatalf("total = %d, want 1200000 (mutual plus copay)", subject.TotalMinor())
	}
	if subject.InsurerID == nil || *subject.InsurerID != *insurerID() {
		t.Fatalf("insurer = %v, want the draft's mutual", subject.InsurerID)
	}
}

func TestNewRefusesAnInvalidDraft(t *testing.T) {
	cases := map[string]struct {
		mutate func(*pricing.Draft)
		field  string
	}{
		"no professional": {
			mutate: func(d *pricing.Draft) { d.ProfessionalUserID = "" },
			field:  "professional_user_id",
		},
		"unknown coverage": {
			mutate: func(d *pricing.Draft) { d.Coverage = "prepaga" },
			field:  "coverage",
		},
		"particular names a mutual": {
			mutate: func(d *pricing.Draft) { d.InsurerID = insurerID() },
			field:  "insurer_id",
		},
		"mutual price without a mutual": {
			mutate: func(d *pricing.Draft) {
				d.Coverage = patient.CoverageInsurer
				d.InsurerID = nil
			},
			field: "insurer_id",
		},
		"mutual price with a blank mutual": {
			mutate: func(d *pricing.Draft) {
				d.Coverage = patient.CoverageInsurer
				blank := ""
				d.InsurerID = &blank
			},
			field: "insurer_id",
		},
		"unknown modality": {
			mutate: func(d *pricing.Draft) { d.Modality = "hibrido" },
			field:  "modality",
		},
		"empty modality": {
			mutate: func(d *pricing.Draft) { d.Modality = "" },
			field:  "modality",
		},
		"unknown currency": {
			mutate: func(d *pricing.Draft) { d.Currency = "ARS " },
			field:  "billing_currency",
		},
		"no start day": {
			mutate: func(d *pricing.Draft) { d.ValidFrom = time.Time{} },
			field:  "valid_from",
		},
		"negative patient share": {
			mutate: func(d *pricing.Draft) { d.PatientMinor = -1 },
			field:  "patient_minor",
		},
		"particular with a mutual share": {
			mutate: func(d *pricing.Draft) { d.InsurerMinor = 5 },
			field:  "insurer_minor",
		},
		"session worth nothing": {
			mutate: func(d *pricing.Draft) { d.PatientMinor = 0 },
			field:  "patient_minor",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			draft := privateDraft()
			tc.mutate(&draft)
			_, err := pricing.New("tar-1", tenant.ID("tenant-1"), draft, now)
			if got := validationField(t, err); got != tc.field {
				t.Fatalf("field = %q, want %q (error: %v)", got, tc.field, err)
			}
		})
	}
}

func TestNewRefusesAnEmptyIdentityOrTenant(t *testing.T) {
	if _, err := pricing.New("", tenant.ID("tenant-1"), privateDraft(), now); err == nil {
		t.Fatal("an empty id must be refused")
	}
	if _, err := pricing.New("tar-1", "", privateDraft(), now); err == nil {
		t.Fatal("an empty tenant must be refused")
	}
}

func TestValidFromIsTheCivilDayAtMidnightUTC(t *testing.T) {
	draft := privateDraft()
	// 23:30 in Buenos Aires on the 1st is already the 2nd in UTC; the price starts the 1st.
	buenosAires := time.FixedZone("ART", -3*60*60)
	draft.ValidFrom = time.Date(2026, 11, 1, 23, 30, 0, 0, buenosAires)

	subject, err := pricing.New("tar-1", tenant.ID("tenant-1"), draft, now)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	want := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if !subject.ValidFrom.Equal(want) {
		t.Fatalf("valid from = %v, want %v", subject.ValidFrom, want)
	}
}

func TestChangeAmountsKeepsTheCoverageRules(t *testing.T) {
	subject, err := pricing.New("tar-1", tenant.ID("tenant-1"), privateDraft(), now)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if _, err := subject.ChangeAmounts(pricing.Amounts{InsurerMinor: 10, PatientMinor: 10}, now); err == nil {
		t.Fatal("a particular price must not gain a mutual share")
	}

	later := now.Add(time.Hour)
	updated, err := subject.ChangeAmounts(pricing.Amounts{PatientMinor: 2_000_000}, later)
	if err != nil {
		t.Fatalf("change amounts: %v", err)
	}
	if updated.PatientMinor != 2_000_000 {
		t.Fatalf("patient minor = %d, want 2000000", updated.PatientMinor)
	}
	if !updated.UpdatedAt.Equal(later) {
		t.Fatalf("updated at = %v, want %v", updated.UpdatedAt, later)
	}
	if !updated.CreatedAt.Equal(subject.CreatedAt) {
		t.Fatal("editing must not move the creation time")
	}
	if !updated.ValidFrom.Equal(subject.ValidFrom) {
		t.Fatal("editing amounts must not move the start day")
	}
}

func TestReconstituteTrustsOnlyAConsistentRow(t *testing.T) {
	subject, err := pricing.New("tar-1", tenant.ID("tenant-1"), insurerDraft(), now)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := pricing.Reconstitute(subject); err != nil {
		t.Fatalf("a row that passed New must come back: %v", err)
	}

	corrupt := subject
	corrupt.Coverage = patient.CoveragePrivate
	if _, err := pricing.Reconstitute(corrupt); err == nil {
		t.Fatal("a particular row that carries a mutual share must be refused")
	}
}

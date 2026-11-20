package insurer_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var now = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func str(value string) *string { return &value }

func newInsurer(t *testing.T, draft insurer.Draft) insurer.Insurer {
	t.Helper()
	subject, err := insurer.New(insurer.ID(uuid.NewString()), tenant.ID(uuid.NewString()), draft, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return subject
}

func TestNewInsurerIsActiveAndTrimmed(t *testing.T) {
	subject := newInsurer(t, insurer.Draft{Name: "  OSDE  "})

	if !subject.IsActive {
		t.Error("a new mutual must be active")
	}
	if subject.Name != "OSDE" {
		t.Errorf("Name = %q, want it trimmed", subject.Name)
	}
	if subject.TaxID != nil {
		t.Errorf("TaxID = %v, want nil when none is given", *subject.TaxID)
	}
	if !subject.CreatedAt.Equal(now) || !subject.UpdatedAt.Equal(now) {
		t.Errorf("timestamps = %v/%v, want %v", subject.CreatedAt, subject.UpdatedAt, now)
	}
}

func TestTaxIDKeepsOnlyTheDigits(t *testing.T) {
	for _, typed := range []string{"30-12345678-9", "30.12345678.9", "30 12345678 9", "30123456789"} {
		subject := newInsurer(t, insurer.Draft{Name: "Swiss Medical", TaxID: str(typed)})
		if subject.TaxID == nil || *subject.TaxID != "30123456789" {
			t.Errorf("TaxID(%q) = %v, want 30123456789", typed, subject.TaxID)
		}
	}
}

func TestNewInsurerValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		draft insurer.Draft
		field string
	}{
		"blank name":          {insurer.Draft{Name: "   "}, "name"},
		"tab-only name":       {insurer.Draft{Name: "\t"}, "name"},
		"tax id too short":    {insurer.Draft{Name: "OSDE", TaxID: str("3012345678")}, "tax_id"},
		"tax id too long":     {insurer.Draft{Name: "OSDE", TaxID: str("301234567890")}, "tax_id"},
		"tax id with letters": {insurer.Draft{Name: "OSDE", TaxID: str("30-1234567A-9")}, "tax_id"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := insurer.New(insurer.ID(uuid.NewString()), tenant.ID(uuid.NewString()), tc.draft, now)

			var validation *insurer.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want a ValidationError", err)
			}
			if validation.Field != tc.field {
				t.Fatalf("field = %q, want %q", validation.Field, tc.field)
			}
		})
	}
}

func TestNewInsurerRejectsEmptyIdentity(t *testing.T) {
	if _, err := insurer.New("", tenant.ID(uuid.NewString()), insurer.Draft{Name: "OSDE"}, now); err == nil {
		t.Fatal("an empty mutual id must be rejected")
	}
	if _, err := insurer.New(insurer.ID(uuid.NewString()), "", insurer.Draft{Name: "OSDE"}, now); err == nil {
		t.Fatal("an empty tenant id must be rejected")
	}
}

func TestDeactivateAndActivate(t *testing.T) {
	subject := newInsurer(t, insurer.Draft{Name: "OSDE"})
	later := now.Add(time.Hour)

	inactive, err := subject.Deactivate(later)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inactive.IsActive || !inactive.UpdatedAt.Equal(later) {
		t.Fatalf("deactivated = (%v, %v), want inactive at %v", inactive.IsActive, inactive.UpdatedAt, later)
	}
	if _, err := inactive.Deactivate(later); !errors.Is(err, insurer.ErrInvalidTransition) {
		t.Fatalf("deactivating twice = %v, want %v", err, insurer.ErrInvalidTransition)
	}

	reactivated, err := inactive.Activate(later)
	if err != nil || !reactivated.IsActive {
		t.Fatalf("activate = (%v, %v), want active", reactivated.IsActive, err)
	}
	if _, err := reactivated.Activate(later); !errors.Is(err, insurer.ErrInvalidTransition) {
		t.Fatalf("activating twice = %v, want %v", err, insurer.ErrInvalidTransition)
	}
}

func TestChangeDetailsKeepsIdentityAndCreation(t *testing.T) {
	subject := newInsurer(t, insurer.Draft{Name: "OSDE"})
	later := now.Add(48 * time.Hour)

	updated, err := subject.ChangeDetails(insurer.Draft{Name: "OSDE Binario", TaxID: str("30-12345678-9")}, later)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.ID != subject.ID || updated.TenantID != subject.TenantID {
		t.Error("ChangeDetails changed the identity")
	}
	if !updated.CreatedAt.Equal(subject.CreatedAt) || !updated.UpdatedAt.Equal(later) {
		t.Errorf("timestamps = %v/%v, want creation kept and update at %v", updated.CreatedAt, updated.UpdatedAt, later)
	}
	if updated.Name != "OSDE Binario" || updated.TaxID == nil || *updated.TaxID != "30123456789" {
		t.Errorf("details = (%q, %v), want the new name and the normalized tax id", updated.Name, updated.TaxID)
	}
}

func TestChangeDetailsRefusesInactiveMutual(t *testing.T) {
	inactive, err := newInsurer(t, insurer.Draft{Name: "OSDE"}).Deactivate(now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := inactive.ChangeDetails(insurer.Draft{Name: "Otra"}, now); !errors.Is(err, insurer.ErrInvalidTransition) {
		t.Fatalf("error = %v, want %v", err, insurer.ErrInvalidTransition)
	}
}

func TestReconstituteRefusesBlankName(t *testing.T) {
	if _, err := insurer.Reconstitute(insurer.Stored{ID: "x", Name: "  "}); err == nil {
		t.Fatal("a stored blank name must not rebuild")
	}
}

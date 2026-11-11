package patient_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var now = time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)

func validDraft() patient.Draft {
	return patient.Draft{
		FullName: "  Ana Pérez  ",
		Country:  "AR",
		Document: &patient.Document{
			Country:    "AR",
			Kind:       "dni",
			Number:     "30.111.222",
			Normalized: "30111222",
		},
	}
}

func newPatient(t *testing.T, draft patient.Draft) patient.Patient {
	t.Helper()
	subject, err := patient.New(patient.ID(uuid.NewString()), tenant.ID(uuid.NewString()), draft, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return subject
}

func TestNewPatient(t *testing.T) {
	subject := newPatient(t, validDraft())

	if !subject.IsActive {
		t.Error("a new patient must be active")
	}
	if subject.FullName != "Ana Pérez" {
		t.Errorf("FullName = %q, want it trimmed", subject.FullName)
	}
	if !subject.CreatedAt.Equal(now) || !subject.UpdatedAt.Equal(now) {
		t.Errorf("timestamps = %v/%v, want %v", subject.CreatedAt, subject.UpdatedAt, now)
	}
	if subject.Document == nil || subject.Document.Normalized != "30111222" {
		t.Errorf("document = %+v, want the normalized DNI", subject.Document)
	}
}

func TestNewPatientValidation(t *testing.T) {
	document := validDraft().Document

	for name, tc := range map[string]struct {
		mutate func(*patient.Draft)
		field  string
	}{
		"blank name": {func(d *patient.Draft) { d.FullName = "   " }, "full_name"},
		// btrim trims spaces and not tabs, and so does the schema constraint: the
		// domain mirrors it so the caller gets a field name instead of a SQLSTATE.
		"tabs are not blank": {func(d *patient.Draft) { d.FullName = "\t" }, ""},
		"name of tabs only":  {func(d *patient.Draft) { d.FullName = "\t\t" }, ""},
		"bad country":        {func(d *patient.Draft) { d.Country = "arg" }, "country"},
		"document country mismatch": {func(d *patient.Draft) {
			d.Document = &patient.Document{Country: "ES", Kind: "dni", Normalized: "12345678Z"}
		}, "document_country"},
		"document without normalized": {func(d *patient.Draft) {
			d.Document = &patient.Document{Country: "AR", Kind: "dni", Number: "30111222"}
		}, "document_number"},
		"tax id without normalized": {func(d *patient.Draft) {
			d.TaxID = &patient.TaxID{Kind: "cuit"}
		}, "tax_id"},
		"birth date in the future": {func(d *patient.Draft) {
			when := now.Add(24 * time.Hour)
			d.BirthDate = &when
		}, "birth_date"},
	} {
		t.Run(name, func(t *testing.T) {
			draft := validDraft()
			draft.Document = document
			tc.mutate(&draft)

			_, err := patient.New(patient.ID(uuid.NewString()), tenant.ID(uuid.NewString()), draft, now)

			if tc.field == "" {
				// A tab-only name is NOT blank for btrim, so the row is accepted. The
				// point of the case is that the domain agrees with the database.
				if err != nil {
					t.Fatalf("error = %v, want the same answer the schema gives", err)
				}
				return
			}

			var validation *patient.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %v, want a ValidationError", err)
			}
			if validation.Field != tc.field {
				t.Fatalf("field = %q, want %q", validation.Field, tc.field)
			}
		})
	}
}

func TestNewPatientRejectsEmptyIdentity(t *testing.T) {
	if _, err := patient.New("", tenant.ID(uuid.NewString()), validDraft(), now); err == nil {
		t.Fatal("an empty patient id must be rejected")
	}
	if _, err := patient.New(patient.ID(uuid.NewString()), "", validDraft(), now); err == nil {
		t.Fatal("an empty tenant id must be rejected")
	}
}

func TestOptionalFieldsCollapse(t *testing.T) {
	empty := "   "
	email := "  Ana@Consultorio.Test "

	draft := validDraft()
	draft.Phone = &empty
	draft.Email = &email
	draft.AdministrativeNotes = &empty

	subject := newPatient(t, draft)

	if subject.Phone != nil {
		t.Errorf("Phone = %q, want nil for a blank value", *subject.Phone)
	}
	if subject.AdministrativeNotes != nil {
		t.Errorf("AdministrativeNotes = %q, want nil for a blank value", *subject.AdministrativeNotes)
	}
	if subject.Email == nil || *subject.Email != "ana@consultorio.test" {
		t.Errorf("Email = %v, want it trimmed and lowercased", subject.Email)
	}
}

func TestPatientLifecycle(t *testing.T) {
	subject := newPatient(t, validDraft())
	later := now.Add(time.Hour)

	inactive, err := subject.Deactivate(later)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inactive.IsActive {
		t.Error("the patient is still active")
	}
	if !inactive.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", inactive.UpdatedAt, later)
	}

	if _, err := inactive.Deactivate(later); !errors.Is(err, patient.ErrInvalidTransition) {
		t.Fatalf("deactivating twice = %v, want %v", err, patient.ErrInvalidTransition)
	}

	reactivated, err := inactive.Activate(later)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reactivated.IsActive {
		t.Error("the patient was not reactivated")
	}
	if _, err := reactivated.Activate(later); !errors.Is(err, patient.ErrInvalidTransition) {
		t.Fatalf("activating twice = %v, want %v", err, patient.ErrInvalidTransition)
	}
}

func TestChangeProfile(t *testing.T) {
	subject := newPatient(t, validDraft())
	later := now.Add(48 * time.Hour)

	edited := validDraft()
	edited.FullName = "Ana María Pérez"
	edited.Country = "AR"

	updated, err := subject.ChangeProfile(edited, later)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.ID != subject.ID || updated.TenantID != subject.TenantID {
		t.Error("ChangeProfile changed the identity")
	}
	if !updated.CreatedAt.Equal(subject.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v untouched", updated.CreatedAt, subject.CreatedAt)
	}
	if !updated.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", updated.UpdatedAt, later)
	}
	if updated.FullName != "Ana María Pérez" {
		t.Errorf("FullName = %q, want the new value", updated.FullName)
	}
}

func TestChangeProfileRefusesInactivePatient(t *testing.T) {
	subject := newPatient(t, validDraft())
	inactive, err := subject.Deactivate(now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := inactive.ChangeProfile(validDraft(), now); !errors.Is(err, patient.ErrInvalidTransition) {
		t.Fatalf("error = %v, want %v", err, patient.ErrInvalidTransition)
	}
}

func TestReconstitute(t *testing.T) {
	subject := newPatient(t, validDraft())

	rebuilt, err := patient.Reconstitute(patient.Stored{
		ID:        subject.ID,
		TenantID:  subject.TenantID,
		Country:   subject.Country,
		FullName:  subject.FullName,
		Document:  subject.Document,
		IsActive:  true,
		CreatedAt: subject.CreatedAt,
		UpdatedAt: subject.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rebuilt.ID != subject.ID || rebuilt.Document == nil || rebuilt.Document.Normalized != "30111222" {
		t.Fatalf("rebuilt = %+v, want the same aggregate", rebuilt)
	}

	if _, err := patient.Reconstitute(patient.Stored{TenantID: subject.TenantID, Country: "AR", FullName: "Ana"}); err == nil {
		t.Fatal("Reconstitute without an id must fail")
	}
}

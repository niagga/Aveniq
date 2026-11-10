package tenant_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

func TestNewTenant(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	id := tenant.ID("0192f3a4-0000-7000-8000-000000000001")

	t.Run("accepts a valid slug and name", func(t *testing.T) {
		t.Parallel()

		got, err := tenant.New(id, "consultorio-sur", "Consultorio Sur", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != id {
			t.Errorf("ID = %q, want %q", got.ID, id)
		}
		if got.Slug != "consultorio-sur" {
			t.Errorf("Slug = %q, want %q", got.Slug, "consultorio-sur")
		}
		if got.Name != "Consultorio Sur" {
			t.Errorf("Name = %q, want %q", got.Name, "Consultorio Sur")
		}
		if got.Status != tenant.StatusTrial {
			t.Errorf("Status = %q, want %q (a new tenant always starts on trial)", got.Status, tenant.StatusTrial)
		}
		if !got.CreatedAt.Equal(now) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
		}
	})

	t.Run("normalizes case and surrounding spaces", func(t *testing.T) {
		t.Parallel()

		got, err := tenant.New(id, "  Consultorio-Sur  ", "   Consultorio Sur   ", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Slug != "consultorio-sur" {
			t.Errorf("Slug = %q, want lowercased %q", got.Slug, "consultorio-sur")
		}
		if got.Name != "Consultorio Sur" {
			t.Errorf("Name = %q, want trimmed %q", got.Name, "Consultorio Sur")
		}
	})

	invalid := []struct {
		name       string
		slug       string
		tenantName string
		wantField  string
	}{
		{name: "empty slug", slug: "", tenantName: "X", wantField: "slug"},
		{name: "slug too short", slug: "ab", tenantName: "X", wantField: "slug"},
		{name: "slug too long", slug: "a" + strings.Repeat("b", 64), tenantName: "X", wantField: "slug"},
		{name: "slug with underscore", slug: "con_sultorio", tenantName: "X", wantField: "slug"},
		{name: "slug with leading dash", slug: "-sur", tenantName: "X", wantField: "slug"},
		{name: "slug with trailing dash", slug: "sur-", tenantName: "X", wantField: "slug"},
		{name: "empty name", slug: "sur", tenantName: "   ", wantField: "name"},
		{name: "name too long", slug: "sur", tenantName: strings.Repeat("n", 121), wantField: "name"},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tenant.New(id, tc.slug, tc.tenantName, now)
			if err == nil {
				t.Fatalf("expected a validation error for %s", tc.name)
			}

			var validationErr *tenant.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *tenant.ValidationError", err)
			}
			if validationErr.Field != tc.wantField {
				t.Errorf("Field = %q, want %q", validationErr.Field, tc.wantField)
			}
		})
	}
}

func TestStatusTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	subject, err := tenant.New(tenant.ID("0192f3a4-0000-7000-8000-000000000002"), "sur", "Sur", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("activates a trial tenant", func(t *testing.T) {
		t.Parallel()

		activated, err := subject.Activate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if activated.Status != tenant.StatusActive {
			t.Errorf("Status = %q, want %q", activated.Status, tenant.StatusActive)
		}
		if subject.Status != tenant.StatusTrial {
			t.Errorf("Activate must not mutate the receiver: Status = %q", subject.Status)
		}
	})

	t.Run("refuses to activate an already active tenant", func(t *testing.T) {
		t.Parallel()

		active := subject
		active.Status = tenant.StatusActive

		if _, err := active.Activate(); !errors.Is(err, tenant.ErrInvalidTransition) {
			t.Fatalf("error = %v, want %v", err, tenant.ErrInvalidTransition)
		}
	})
}

package iam_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var (
	testTenantID = tenant.ID("0192f3a4-0000-7000-8000-0000000000aa")
	testUserID   = iam.UserID("0192f3a4-0000-7000-8000-000000000001")
	testNow      = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
)

func TestNewUser(t *testing.T) {
	t.Parallel()

	t.Run("accepts a valid user and applies the defaults", func(t *testing.T) {
		t.Parallel()

		got, err := iam.NewUser(testUserID, testTenantID, "ana@consultorio.test", "argon2id$hash", "Ana Pérez", iam.RoleProfessional, testNow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != testUserID {
			t.Errorf("ID = %q, want %q", got.ID, testUserID)
		}
		if got.TenantID != testTenantID {
			t.Errorf("TenantID = %q, want %q", got.TenantID, testTenantID)
		}
		if got.Email != "ana@consultorio.test" {
			t.Errorf("Email = %q, want %q", got.Email, "ana@consultorio.test")
		}
		if got.Role != iam.RoleProfessional {
			t.Errorf("Role = %q, want %q", got.Role, iam.RoleProfessional)
		}
		if !got.IsActive {
			t.Errorf("IsActive = false, want true (a new user starts active)")
		}
		if got.EmailVerifiedAt != nil {
			t.Errorf("EmailVerifiedAt = %v, want nil (a new user has an unverified email)", got.EmailVerifiedAt)
		}
		if !got.CreatedAt.Equal(testNow) || !got.UpdatedAt.Equal(testNow) {
			t.Errorf("CreatedAt/UpdatedAt = %v/%v, want %v", got.CreatedAt, got.UpdatedAt, testNow)
		}
	})

	t.Run("normalizes the email to the canonical form the database enforces", func(t *testing.T) {
		t.Parallel()

		got, err := iam.NewUser(testUserID, testTenantID, "  Ana@Consultorio.Test ", "argon2id$hash", "Ana Pérez", iam.RoleProfessional, testNow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Email != "ana@consultorio.test" {
			t.Errorf("Email = %q, want lowercased and space-trimmed %q", got.Email, "ana@consultorio.test")
		}
	})

}

func TestNewUserValidation(t *testing.T) {
	t.Parallel()

	invalid := []struct {
		name      string
		id        iam.UserID
		email     string
		hash      string
		fullName  string
		role      iam.RoleCode
		wantField string
	}{
		{name: "empty id", id: "", email: "ana@consultorio.test", hash: "argon2id$hash", fullName: "Ana", role: iam.RoleOwner, wantField: "id"},
		{name: "empty email", id: testUserID, email: "", hash: "argon2id$hash", fullName: "Ana", role: iam.RoleOwner, wantField: "email"},
		{name: "email without at sign", id: testUserID, email: "ana.test", hash: "argon2id$hash", fullName: "Ana", role: iam.RoleOwner, wantField: "email"},
		{name: "email with two at signs", id: testUserID, email: "a@b@c.test", hash: "argon2id$hash", fullName: "Ana", role: iam.RoleOwner, wantField: "email"},
		{name: "email with a space in the local part", id: testUserID, email: "a b@c.test", hash: "argon2id$hash", fullName: "Ana", role: iam.RoleOwner, wantField: "email"},
		{name: "email with a tab (btrim only removes spaces)", id: testUserID, email: "\ta@c.test", hash: "argon2id$hash", fullName: "Ana", role: iam.RoleOwner, wantField: "email"},
		{name: "blank password hash", id: testUserID, email: "ana@c.test", hash: "   ", fullName: "Ana", role: iam.RoleOwner, wantField: "password_hash"},
		{name: "empty password hash", id: testUserID, email: "ana@c.test", hash: "", fullName: "Ana", role: iam.RoleOwner, wantField: "password_hash"},
		{name: "blank full name", id: testUserID, email: "ana@c.test", hash: "argon2id$hash", fullName: "   ", role: iam.RoleOwner, wantField: "full_name"},
		{name: "unknown role", id: testUserID, email: "ana@c.test", hash: "argon2id$hash", fullName: "Ana", role: "superadmin", wantField: "role"},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := iam.NewUser(tc.id, testTenantID, tc.email, tc.hash, tc.fullName, tc.role, testNow)
			if err == nil {
				t.Fatalf("expected a validation error for %s", tc.name)
			}

			var validationErr *iam.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *iam.ValidationError", err)
			}
			if validationErr.Field != tc.wantField {
				t.Errorf("Field = %q, want %q", validationErr.Field, tc.wantField)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "mixed case and surrounding spaces", raw: "  Ana.Perez@Consultorio.TEST ", want: "ana.perez@consultorio.test"},
		{name: "already canonical", raw: "ana@consultorio.test", want: "ana@consultorio.test"},
		{name: "empty", raw: "", wantErr: true},
		{name: "only spaces", raw: "   ", wantErr: true},
		{name: "no at sign", raw: "ana.test", wantErr: true},
		{name: "two at signs", raw: "a@b@c", wantErr: true},
		{name: "space in local part", raw: "a b@c.test", wantErr: true},
		{name: "space in domain", raw: "a@c .test", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := iam.NormalizeEmail(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeEmail(%q) = %q, want an error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeEmail(%q) unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestReconstituteUser(t *testing.T) {
	t.Parallel()

	verified := testNow.Add(24 * time.Hour)

	t.Run("rebuilds valid persisted state", func(t *testing.T) {
		t.Parallel()

		got, err := iam.ReconstituteUser(testUserID, testTenantID, "ana@c.test", "argon2id$hash", "Ana", iam.RoleAdmin, false, &verified, nil, testNow, verified)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Role != iam.RoleAdmin || got.IsActive {
			t.Errorf("Role/IsActive = %q/%v, want admin/false", got.Role, got.IsActive)
		}
		if got.EmailVerifiedAt == nil || !got.EmailVerifiedAt.Equal(verified) {
			t.Errorf("EmailVerifiedAt = %v, want %v", got.EmailVerifiedAt, verified)
		}
	})

	invalid := []struct {
		name      string
		email     string
		role      iam.RoleCode
		wantField string
	}{
		{name: "stored email is not lowercase", email: "Ana@c.test", role: iam.RoleOwner, wantField: "email"},
		{name: "stored email has surrounding spaces", email: " ana@c.test", role: iam.RoleOwner, wantField: "email"},
		{name: "stored email has no at sign", email: "ana.c.test", role: iam.RoleOwner, wantField: "email"},
		{name: "stored role is unknown", email: "ana@c.test", role: "ceo", wantField: "role"},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := iam.ReconstituteUser(testUserID, testTenantID, tc.email, "argon2id$hash", "Ana", tc.role, true, nil, nil, testNow, testNow)
			if err == nil {
				t.Fatalf("expected a validation error for %s", tc.name)
			}
			var validationErr *iam.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *iam.ValidationError", err)
			}
			if validationErr.Field != tc.wantField {
				t.Errorf("Field = %q, want %q", validationErr.Field, tc.wantField)
			}
		})
	}
}

func TestUserTransitions(t *testing.T) {
	t.Parallel()

	newUser := func(t *testing.T) iam.User {
		t.Helper()
		subject, err := iam.NewUser(testUserID, testTenantID, "ana@c.test", "argon2id$hash", "Ana", iam.RoleAssistant, testNow)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return subject
	}

	later := testNow.Add(time.Hour)

	t.Run("deactivates an active user and stamps the update time", func(t *testing.T) {
		t.Parallel()

		subject := newUser(t)
		got, err := subject.Deactivate(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.IsActive {
			t.Errorf("IsActive = true, want false")
		}
		if !got.UpdatedAt.Equal(later) {
			t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
		}
		if !subject.IsActive {
			t.Errorf("Deactivate must not mutate the receiver: IsActive = %v", subject.IsActive)
		}
	})

	t.Run("refuses to deactivate an already inactive user", func(t *testing.T) {
		t.Parallel()

		subject := newUser(t)
		inactive, err := subject.Deactivate(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := inactive.Deactivate(later); !errors.Is(err, iam.ErrInvalidTransition) {
			t.Fatalf("error = %v, want %v", err, iam.ErrInvalidTransition)
		}
	})

	t.Run("reactivates an inactive user", func(t *testing.T) {
		t.Parallel()

		subject := newUser(t)
		inactive, err := subject.Deactivate(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := inactive.Activate(later.Add(time.Hour))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.IsActive {
			t.Errorf("IsActive = false, want true")
		}
	})

	t.Run("refuses to activate an already active user", func(t *testing.T) {
		t.Parallel()

		subject := newUser(t)
		if _, err := subject.Activate(later); !errors.Is(err, iam.ErrInvalidTransition) {
			t.Fatalf("error = %v, want %v", err, iam.ErrInvalidTransition)
		}
	})

	t.Run("changes the role", func(t *testing.T) {
		t.Parallel()

		subject := newUser(t)
		got, err := subject.ChangeRole(iam.RoleAdmin, later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Role != iam.RoleAdmin {
			t.Errorf("Role = %q, want %q", got.Role, iam.RoleAdmin)
		}
		if subject.Role != iam.RoleAssistant {
			t.Errorf("ChangeRole must not mutate the receiver: Role = %q", subject.Role)
		}
	})

	t.Run("refuses an unknown role", func(t *testing.T) {
		t.Parallel()

		subject := newUser(t)
		if _, err := subject.ChangeRole("ceo", later); !errors.Is(err, iam.ErrInvalidTransition) {
			t.Fatalf("error = %v, want %v", err, iam.ErrInvalidTransition)
		}
	})

	t.Run("verifies the email once", func(t *testing.T) {
		t.Parallel()

		subject := newUser(t)
		got, err := subject.VerifyEmail(later)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.EmailVerifiedAt == nil || !got.EmailVerifiedAt.Equal(later) {
			t.Errorf("EmailVerifiedAt = %v, want %v", got.EmailVerifiedAt, later)
		}
		if _, err := got.VerifyEmail(later.Add(time.Hour)); !errors.Is(err, iam.ErrInvalidTransition) {
			t.Fatalf("error = %v, want %v (email verification is one-way)", err, iam.ErrInvalidTransition)
		}
	})
}

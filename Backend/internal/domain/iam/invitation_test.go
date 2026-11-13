package iam_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var invitationNow = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

const invitationTenant = tenant.ID("11111111-1111-4111-8111-111111111111")

func newTestInvitation(t *testing.T, email string, role iam.RoleCode) iam.Invitation {
	t.Helper()
	invitation, err := iam.NewInvitation("inv-1", invitationTenant, email, role, "hash-abc", "owner-1", invitationNow)
	if err != nil {
		t.Fatalf("NewInvitation: %v", err)
	}
	return invitation
}

func TestNewInvitation(t *testing.T) {
	t.Run("canonicalises the email and sets the seven-day expiry", func(t *testing.T) {
		invitation := newTestInvitation(t, "  Ana@Practica.TEST ", iam.RoleProfessional)

		if invitation.Email != "ana@practica.test" {
			t.Fatalf("email = %q, want the canonical form", invitation.Email)
		}
		if got := invitation.ExpiresAt.Sub(invitationNow); got != iam.InvitationTTL {
			t.Fatalf("expiry after %v, want %v", got, iam.InvitationTTL)
		}
	})

	t.Run("refuses an unknown role", func(t *testing.T) {
		_, err := iam.NewInvitation("inv-1", invitationTenant, "ana@practica.test", iam.RoleCode("superadmin"), "hash", "owner-1", invitationNow)
		assertValidation(t, err, "role")
	})

	t.Run("refuses a blank token hash", func(t *testing.T) {
		_, err := iam.NewInvitation("inv-1", invitationTenant, "ana@practica.test", iam.RoleAssistant, "   ", "owner-1", invitationNow)
		assertValidation(t, err, "token_hash")
	})

	t.Run("refuses an invitation without an inviter", func(t *testing.T) {
		_, err := iam.NewInvitation("inv-1", invitationTenant, "ana@practica.test", iam.RoleAssistant, "hash", "", invitationNow)
		assertValidation(t, err, "invited_by")
	})

	t.Run("refuses a malformed email", func(t *testing.T) {
		_, err := iam.NewInvitation("inv-1", invitationTenant, "ana@@practica.test", iam.RoleAssistant, "hash", "owner-1", invitationNow)
		assertValidation(t, err, "email")
	})
}

func TestInvitationUsable(t *testing.T) {
	invitation := newTestInvitation(t, "ana@practica.test", iam.RoleAssistant)

	t.Run("usable before expiry", func(t *testing.T) {
		if err := invitation.Usable(invitationNow.Add(time.Hour)); err != nil {
			t.Fatalf("Usable = %v, want nil", err)
		}
	})

	t.Run("expired at the exact expiry instant", func(t *testing.T) {
		err := invitation.Usable(invitation.ExpiresAt)
		if !errors.Is(err, iam.ErrInvitationUnusable) {
			t.Fatalf("Usable at expiry = %v, want ErrInvitationUnusable", err)
		}
	})

	t.Run("accepted invitations are not usable again", func(t *testing.T) {
		accepted, err := invitation.Accept(invitationNow.Add(time.Hour))
		if err != nil {
			t.Fatalf("Accept: %v", err)
		}
		if err := accepted.Usable(invitationNow.Add(2 * time.Hour)); !errors.Is(err, iam.ErrInvitationUnusable) {
			t.Fatalf("Usable after accept = %v, want ErrInvitationUnusable", err)
		}
		if invitation.AcceptedAt != nil {
			t.Fatal("Accept mutated the original value; the aggregate must be a value transition")
		}
	})

	t.Run("an expired invitation cannot be accepted", func(t *testing.T) {
		if _, err := invitation.Accept(invitation.ExpiresAt.Add(time.Second)); !errors.Is(err, iam.ErrInvitationUnusable) {
			t.Fatalf("Accept after expiry = %v, want ErrInvitationUnusable", err)
		}
	})
}

func TestAuthorizeInvitation(t *testing.T) {
	cases := []struct {
		name  string
		actor iam.RoleCode
		role  iam.RoleCode
		allow bool
	}{
		{"owner invites owner", iam.RoleOwner, iam.RoleOwner, true},
		{"admin invites staff", iam.RoleAdmin, iam.RoleProfessional, true},
		{"admin invites admin", iam.RoleAdmin, iam.RoleAdmin, true},
		{"admin cannot invite owner", iam.RoleAdmin, iam.RoleOwner, false},
		{"professional cannot invite owner", iam.RoleProfessional, iam.RoleOwner, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := iam.AuthorizeInvitation(iam.Actor{ID: "actor", Role: tc.actor}, tc.role)
			if tc.allow && err != nil {
				t.Fatalf("AuthorizeInvitation = %v, want nil", err)
			}
			if !tc.allow && !errors.Is(err, iam.ErrChangeNotAllowed) {
				t.Fatalf("AuthorizeInvitation = %v, want ErrChangeNotAllowed", err)
			}
		})
	}
}

func TestReconstituteInvitationAcceptsAVanishedInviter(t *testing.T) {
	// invited_by is ON DELETE SET NULL: the pending offer survives its inviter.
	invitation, err := iam.ReconstituteInvitation(
		"inv-1", invitationTenant, "ana@practica.test", iam.RoleAssistant, "hash", "",
		invitationNow.Add(time.Hour), nil, invitationNow,
	)
	if err != nil {
		t.Fatalf("ReconstituteInvitation without inviter: %v", err)
	}
	if err := invitation.Usable(invitationNow); err != nil {
		t.Fatalf("a pending invitation without inviter must stay usable: %v", err)
	}
}

func TestReconstituteInvitationRejectsBrokenRows(t *testing.T) {
	_, err := iam.ReconstituteInvitation(
		"inv-1", invitationTenant, "Ana@Practica.test", iam.RoleAssistant, "hash", "",
		invitationNow.Add(time.Hour), nil, invitationNow,
	)
	assertValidation(t, err, "email")
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"eleven characters are refused", strings.Repeat("a", 11), true},
		{"twelve characters are the minimum", strings.Repeat("a", 12), false},
		{"length counts runes, not bytes", strings.Repeat("ñ", 12), false},
		{"256 characters are the maximum", strings.Repeat("a", 256), false},
		{"257 characters are refused", strings.Repeat("a", 257), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := iam.ValidatePassword(tc.password)
			if tc.wantErr {
				assertValidation(t, err, "password")
				return
			}
			if err != nil {
				t.Fatalf("ValidatePassword = %v, want nil", err)
			}
		})
	}
}

func assertValidation(t *testing.T, err error, field string) {
	t.Helper()
	var validation *iam.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want a ValidationError on %q", err, field)
	}
	if validation.Field != field {
		t.Fatalf("ValidationError field = %q, want %q", validation.Field, field)
	}
}

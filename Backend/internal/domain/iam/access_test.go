package iam_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

func memberOf(t *testing.T, id string, role iam.RoleCode) iam.User {
	t.Helper()
	user, err := iam.NewUser(iam.UserID(id), tenant.ID("11111111-1111-1111-1111-111111111111"),
		id+"@practica.test", "hash", "Persona "+id, role, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("cannot build the user: %v", err)
	}
	return user
}

func TestAuthorizeChange(t *testing.T) {
	t.Parallel()

	const (
		ownerID = "00000000-0000-0000-0000-00000000000a"
		adminID = "00000000-0000-0000-0000-00000000000b"
		profID  = "00000000-0000-0000-0000-00000000000c"
		otherID = "00000000-0000-0000-0000-00000000000d"
	)

	owner := iam.Actor{ID: iam.UserID(ownerID), Role: iam.RoleOwner}
	admin := iam.Actor{ID: iam.UserID(adminID), Role: iam.RoleAdmin}

	cases := map[string]struct {
		actor   iam.Actor
		target  iam.User
		newRole iam.RoleCode
		allowed bool
	}{
		"an owner changes a professional to assistant": {
			owner, memberOf(t, profID, iam.RoleProfessional), iam.RoleAssistant, true,
		},
		"an owner grants the owner role to someone else": {
			owner, memberOf(t, otherID, iam.RoleProfessional), iam.RoleOwner, true,
		},
		"an admin changes a professional to assistant": {
			admin, memberOf(t, profID, iam.RoleProfessional), iam.RoleAssistant, true,
		},
		"an admin cannot grant the owner role": {
			admin, memberOf(t, profID, iam.RoleProfessional), iam.RoleOwner, false,
		},
		"an admin cannot demote an owner": {
			admin, memberOf(t, ownerID, iam.RoleOwner), iam.RoleAdmin, false,
		},
		"nobody changes their own access, even an owner": {
			owner, memberOf(t, ownerID, iam.RoleOwner), iam.RoleAdmin, false,
		},
		"an admin cannot change their own role": {
			admin, memberOf(t, adminID, iam.RoleAdmin), iam.RoleProfessional, false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := iam.AuthorizeChange(tc.actor, tc.target, tc.newRole)
			if tc.allowed && err != nil {
				t.Fatalf("AuthorizeChange = %v, want nil", err)
			}
			if !tc.allowed && !errors.Is(err, iam.ErrChangeNotAllowed) {
				t.Fatalf("AuthorizeChange = %v, want ErrChangeNotAllowed", err)
			}
		})
	}
}

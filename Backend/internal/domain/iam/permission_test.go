package iam_test

import (
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

func TestPermissionCatalogIsDistinctAndValid(t *testing.T) {
	t.Parallel()

	seen := map[iam.PermissionCode]bool{}
	for _, code := range iam.Catalog() {
		if seen[code] {
			t.Errorf("permission %q appears twice in Catalog", code)
		}
		seen[code] = true
		if !code.Valid() {
			t.Errorf("Catalog entry %q is not Valid", code)
		}
	}

	if iam.PermissionCode("patients:delete").Valid() {
		t.Error("an unknown permission must not be Valid")
	}
	if iam.PermissionCode("").Valid() {
		t.Error("the empty permission must not be Valid")
	}
}

func TestAllowsReplacesTheCoarseRoleGuard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		role       iam.RoleCode
		permission iam.PermissionCode
		want       bool
	}{
		{iam.RoleOwner, iam.PermClinicalReadAny, true},
		{iam.RoleOwner, iam.PermBillingVoid, true},
		{iam.RoleAdmin, iam.PermPatientsRead, true},
		{iam.RoleAdmin, iam.PermScheduleWrite, true},
		{iam.RoleAdmin, iam.PermClinicalReadOwn, false},
		{iam.RoleAdmin, iam.PermClinicalReadAny, false},
		{iam.RoleProfessional, iam.PermClinicalReadOwn, true},
		{iam.RoleProfessional, iam.PermClinicalGrant, true},
		{iam.RoleProfessional, iam.PermClinicalReadAny, false},
		{iam.RoleProfessional, iam.PermBillingRead, false},
		{iam.RoleAssistant, iam.PermPatientsRead, true},
		{iam.RoleAssistant, iam.PermScheduleWrite, true},
		{iam.RoleAssistant, iam.PermClinicalReadOwn, false},
		{iam.RoleAssistant, iam.PermBillingRead, false},
		{iam.RoleAccountant, iam.PermBillingIssue, true},
		{iam.RoleAccountant, iam.PermPatientsRead, false},
		{iam.RoleAccountant, iam.PermScheduleRead, false},
		{iam.RoleAccountant, iam.PermTenantRead, false},
		{"ceo", iam.PermPatientsRead, false},
		{iam.RoleOwner, "patients:delete", false},
	}

	for _, tc := range cases {
		got := iam.Allows(tc.role, tc.permission)
		if got != tc.want {
			t.Errorf("Allows(%q, %q) = %v, want %v", tc.role, tc.permission, got, tc.want)
		}
	}
}

func TestAllowsAnyIsTheHttpGuard(t *testing.T) {
	t.Parallel()

	if !iam.AllowsAny(iam.RoleProfessional, iam.PermClinicalReadOwn, iam.PermClinicalReadAny) {
		t.Fatal("a professional must pass a clinical read that accepts own or any")
	}
	if iam.AllowsAny(iam.RoleAdmin, iam.PermClinicalReadOwn, iam.PermClinicalReadAny) {
		t.Fatal("an admin must not pass a clinical read: that is why the matrix exists")
	}
	if iam.AllowsAny(iam.RoleOwner, iam.PermClinicalReadOwn, iam.PermClinicalReadAny) {
		// owner has every permission, including both.
	} else {
		t.Fatal("the owner must pass every clinical read")
	}
}

func TestGrantsDoNotListTheOwner(t *testing.T) {
	t.Parallel()

	if _, listed := iam.Grants[iam.RoleOwner]; listed {
		t.Fatal("owner is granted every catalog row by definition; listing it would drift when a permission is added")
	}
	for role, perms := range iam.Grants {
		if !role.Valid() {
			t.Errorf("Grants lists unknown role %q", role)
		}
		seen := map[iam.PermissionCode]bool{}
		for _, perm := range perms {
			if !perm.Valid() {
				t.Errorf("role %q is granted unknown permission %q", role, perm)
			}
			if seen[perm] {
				t.Errorf("role %q is granted %q twice", role, perm)
			}
			seen[perm] = true
		}
	}
}

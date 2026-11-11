package iam_test

import (
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// TestRoleCodeValid pins the five seeded roles: any code outside them must be
// rejected by the domain before it can reach the roles.role_code foreign key.
func TestRoleCodeValid(t *testing.T) {
	t.Parallel()

	known := []iam.RoleCode{
		iam.RoleOwner,
		iam.RoleAdmin,
		iam.RoleProfessional,
		iam.RoleAssistant,
		iam.RoleAccountant,
	}

	for _, role := range known {
		if !role.Valid() {
			t.Errorf("RoleCode %q must be valid: the seed drift guard depends on every constant being accepted", role)
		}
	}

	unknown := []iam.RoleCode{"", "superadmin", "OWNER", "owner ", "ceo", "root"}
	for _, role := range unknown {
		if role.Valid() {
			t.Errorf("RoleCode %q must not be valid", role)
		}
	}
}

// TestRoleCodesAreDistinct guards against two constants sharing one code, which
// would make Valid() pass for a role the catalog does not actually contain.
func TestRoleCodesAreDistinct(t *testing.T) {
	t.Parallel()

	all := []iam.RoleCode{
		iam.RoleOwner,
		iam.RoleAdmin,
		iam.RoleProfessional,
		iam.RoleAssistant,
		iam.RoleAccountant,
	}

	seen := make(map[iam.RoleCode]bool, len(all))
	for _, role := range all {
		if seen[role] {
			t.Errorf("RoleCode %q is declared by two constants", role)
		}
		seen[role] = true
	}
}

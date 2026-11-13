package iam

// PermissionCode is a row of the global permission catalog. Codes are stable
// API surface: they appear in the matrix, in the database, and eventually in
// OpenAPI security. A code that is not here is not a permission.
type PermissionCode string

// Catalog seeded by migrations 0002 and 0010. The drift test proves these
// constants and the `permissions` table never disagree in either direction.
const (
	PermUsersRead        PermissionCode = "users:read"
	PermUsersWrite       PermissionCode = "users:write"
	PermUsersManage      PermissionCode = "users:manage"
	PermUsersDirectory   PermissionCode = "users:directory"
	PermPatientsRead     PermissionCode = "patients:read"
	PermPatientsWrite    PermissionCode = "patients:write"
	PermPatientsAssign   PermissionCode = "patients:assign"
	PermClinicalReadOwn  PermissionCode = "clinical:read:own"
	PermClinicalWriteOwn PermissionCode = "clinical:write:own"
	PermClinicalReadAny  PermissionCode = "clinical:read:any"
	PermClinicalGrant    PermissionCode = "clinical:grant"
	PermScheduleRead     PermissionCode = "schedule:read"
	PermScheduleWrite    PermissionCode = "schedule:write"
	PermBillingRead      PermissionCode = "billing:read"
	PermBillingIssue     PermissionCode = "billing:issue"
	PermBillingVoid      PermissionCode = "billing:void"
	PermReportsRead      PermissionCode = "reports:read"
	PermTenantRead       PermissionCode = "tenant:read"
	PermTenantSettings   PermissionCode = "tenant:settings"
)

// Catalog is every permission the platform knows. Owner is granted all of them
// by definition; the other roles are listed in Grants.
func Catalog() []PermissionCode {
	return []PermissionCode{
		PermUsersRead,
		PermUsersWrite,
		PermUsersManage,
		PermUsersDirectory,
		PermPatientsRead,
		PermPatientsWrite,
		PermPatientsAssign,
		PermClinicalReadOwn,
		PermClinicalWriteOwn,
		PermClinicalReadAny,
		PermClinicalGrant,
		PermScheduleRead,
		PermScheduleWrite,
		PermBillingRead,
		PermBillingIssue,
		PermBillingVoid,
		PermReportsRead,
		PermTenantRead,
		PermTenantSettings,
	}
}

// Valid reports whether the code is one of the seeded permissions.
func (p PermissionCode) Valid() bool {
	for _, known := range Catalog() {
		if p == known {
			return true
		}
	}
	return false
}

// Grants is the matrix for every role except owner. Owner is not listed: it
// holds every permission, including ones added later, without a second row.
//
// Instance-level rules (which patient a professional may treat, a grant of HC
// to another professional) are not this map. This map only answers "does this
// role have this verb on this domain at all".
var Grants = map[RoleCode][]PermissionCode{
	RoleAdmin: {
		PermUsersRead, PermUsersWrite, PermUsersManage,
		PermPatientsRead, PermPatientsWrite, PermPatientsAssign,
		PermScheduleRead, PermScheduleWrite,
		PermBillingRead, PermReportsRead,
		PermTenantRead, PermTenantSettings,
	},
	RoleProfessional: {
		PermPatientsRead, PermPatientsWrite,
		PermClinicalReadOwn, PermClinicalWriteOwn, PermClinicalGrant,
		PermScheduleRead, PermScheduleWrite,
		PermTenantRead, PermUsersDirectory,
	},
	RoleAssistant: {
		PermPatientsRead, PermPatientsWrite,
		PermScheduleRead, PermScheduleWrite,
		PermTenantRead, PermUsersDirectory,
	},
	RoleAccountant: {
		PermBillingRead, PermBillingIssue, PermBillingVoid,
		PermReportsRead,
	},
}

// Allows reports whether the role is granted the permission in the platform
// matrix. Unknown roles and unknown permissions fail closed.
func Allows(role RoleCode, permission PermissionCode) bool {
	if !role.Valid() || !permission.Valid() {
		return false
	}
	if role == RoleOwner {
		return true
	}
	for _, granted := range Grants[role] {
		if granted == permission {
			return true
		}
	}
	return false
}

// AllowsAny is the HTTP guard's question: the caller needs one of these.
func AllowsAny(role RoleCode, permissions ...PermissionCode) bool {
	for _, permission := range permissions {
		if Allows(role, permission) {
			return true
		}
	}
	return false
}

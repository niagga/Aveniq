package iam

// RoleCode is the typed identity of an entry of the global role catalog.
// Roles are platform-wide (decision L6): the same code means the same set of
// permissions for every tenant, so the domain can validate it without reading
// the database.
type RoleCode string

// The five roles seeded by migration 0002. The catalog drift test proves these
// constants and the `roles` table never disagree in either direction.
const (
	// RoleOwner has every permission, including the clinical ones.
	RoleOwner RoleCode = "owner"
	// RoleAdmin manages users, patients and settings. It deliberately has no
	// clinical:* permission: the person who runs the system must not be able
	// to read psychotherapy notes.
	RoleAdmin RoleCode = "admin"
	// RoleProfessional is the treating professional: own patients, own entries.
	RoleProfessional RoleCode = "professional"
	// RoleAssistant is the front desk: patient register and billing overview.
	RoleAssistant RoleCode = "assistant"
	// RoleAccountant issues invoices and reads reports.
	RoleAccountant RoleCode = "accountant"
)

// Valid reports whether the code is one of the seeded roles.
func (r RoleCode) Valid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleProfessional, RoleAssistant, RoleAccountant:
		return true
	default:
		return false
	}
}

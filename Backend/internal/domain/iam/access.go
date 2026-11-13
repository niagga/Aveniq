package iam

import "fmt"

// Actor is who asks for a change to another user: the identity the access token
// carries. Its role decides what the change may touch; the route's permission
// (users:manage) decides whether the change is attempted at all.
type Actor struct {
	ID   UserID
	Role RoleCode
}

// AuthorizeChange decides whether actor may move target to newRole (or deactivate
// or reactivate it, in which case newRole is the target's current role).
//
// Two rules, both about who can end up locking the practice out:
//
//  1. Nobody changes their own access. An owner who demotes themselves can leave
//     the practice without an owner, and no other rule can repair that.
//  2. Only an owner grants or touches the owner role. An admin manages the staff
//     below them; they do not hand out or take away the top role.
//
// Together these keep at least one owner in the practice: the only owner cannot
// demote or deactivate themselves, and no one else can demote an owner who is not
// the one asking.
func AuthorizeChange(actor Actor, target User, newRole RoleCode) error {
	if actor.ID == target.ID {
		return fmt.Errorf("%w: a user cannot change their own access", ErrChangeNotAllowed)
	}
	if actor.Role != RoleOwner && (target.Role == RoleOwner || newRole == RoleOwner) {
		return fmt.Errorf("%w: only an owner grants or changes the owner role", ErrChangeNotAllowed)
	}
	return nil
}

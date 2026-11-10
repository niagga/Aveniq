// Package idgen provides the identity generator the application layer depends on.
//
// UUIDv7 is used because identifiers are time-ordered: primary-key indexes stay
// compact and inserts remain append-mostly, unlike random UUIDv4 keys.
package idgen

import "github.com/google/uuid"

// UUIDGenerator generates time-ordered UUIDv7 identifiers.
type UUIDGenerator struct{}

// NewID returns a new identifier as its canonical string form.
func (UUIDGenerator) NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails when the system entropy source fails. Falling back to
		// a v4 identifier keeps the process alive; the caller cannot do better.
		return uuid.NewString()
	}
	return id.String()
}

package iam

import "unicode/utf8"

// Password length bounds. The minimum is the only composition rule: length
// protects better than character classes, and the invitee chooses the password
// once, so a rule that forces noise into it buys little. The maximum keeps a
// hostile request from making the hasher work on megabytes.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 256
)

// ValidatePassword checks a plaintext password before it is hashed. It answers a
// ValidationError on the password field, which the API reports as 422.
func ValidatePassword(raw string) error {
	length := utf8.RuneCountInString(raw)
	switch {
	case length < MinPasswordLength:
		return &ValidationError{Field: "password", Message: "must have at least 12 characters"}
	case length > MaxPasswordLength:
		return &ValidationError{Field: "password", Message: "must have at most 256 characters"}
	}
	return nil
}

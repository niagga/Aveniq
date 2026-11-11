// Package authn holds the authentication primitives: password hashing and the
// two token formats the API issues.
//
// It is deliberately free of transport, storage and domain types: it knows
// nothing about HTTP, pgx or iam, which is what makes it testable without a
// database or a server. Decisions 2, 5 and 6 of L11 live here.
package authn

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Sentinel errors the delivery layer maps onto transport codes.
var (
	// ErrEmptyPassword reports a hash request with nothing to hash.
	ErrEmptyPassword = errors.New("authn: password must not be empty")
	// ErrMalformedHash reports a stored hash this package cannot parse. It is
	// distinct from ErrPasswordMismatch on purpose: a corrupt row is an incident,
	// a wrong password is a login attempt, and they must not share a log line.
	ErrMalformedHash = errors.New("authn: malformed password hash")
	// ErrUnsupportedHash reports a well-formed PHC string for another algorithm.
	ErrUnsupportedHash = errors.New("authn: unsupported password hash algorithm")
	// ErrPasswordMismatch reports a wrong password.
	ErrPasswordMismatch = errors.New("authn: password does not match")
)

const (
	argon2idAlgorithm = "argon2id"
	phcPrefix         = "$" + argon2idAlgorithm + "$"

	// Bounds applied both when minting and when parsing a stored hash. Parsing
	// takes the costs from the row (decision 6), so the row is also an input: a
	// tampered m=4000000 would otherwise make one login allocate 4 GiB.
	minMemoryKiB   uint32 = 8 * 1024
	maxMemoryKiB   uint32 = 1024 * 1024
	minSaltLength  uint32 = 8
	maxSaltLength  uint32 = 64
	minKeyLength   uint32 = 16
	maxKeyLength   uint32 = 64
	maxIterations  uint32 = 10
	maxParallelism uint8  = 64
)

// Argon2idParams are the cost parameters of a hash. They travel inside every
// stored hash, so raising them never invalidates existing passwords.
type Argon2idParams struct {
	// MemoryKiB is the memory cost, in kibibytes.
	MemoryKiB uint32
	// Iterations is the time cost.
	Iterations uint32
	// Parallelism is the number of lanes.
	Parallelism uint8
	// SaltLength is the number of random bytes per hash.
	SaltLength uint32
	// KeyLength is the length of the derived key.
	KeyLength uint32
}

// DefaultArgon2idParams returns the agreed cost: m=64 MiB, t=3, p=4, a 16-byte
// salt and a 32-byte key (decision 6 of L11).
func DefaultArgon2idParams() Argon2idParams {
	return Argon2idParams{
		MemoryKiB:   64 * 1024,
		Iterations:  3,
		Parallelism: 4,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// PasswordHasher mints and verifies argon2id hashes in the standard PHC encoding.
type PasswordHasher struct {
	params Argon2idParams
}

// NewPasswordHasher validates the cost once, so a misconfiguration fails at
// startup instead of on the first login.
func NewPasswordHasher(params Argon2idParams) (*PasswordHasher, error) {
	if err := validateParams(params); err != nil {
		return nil, err
	}
	return &PasswordHasher{params: params}, nil
}

// Hash derives a PHC-encoded argon2id hash with a fresh random salt.
func (h *PasswordHasher) Hash(password string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}

	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("authn: cannot read random salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		h.params.Iterations,
		h.params.MemoryKiB,
		h.params.Parallelism,
		h.params.KeyLength,
	)

	return fmt.Sprintf(
		"%sv=%d$m=%d,t=%d,p=%d$%s$%s",
		phcPrefix,
		argon2.Version,
		h.params.MemoryKiB,
		h.params.Iterations,
		h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify checks a password against a stored hash, using the costs stored in that
// hash rather than the hasher's own. Raising the production cost therefore keeps
// every existing password verifiable, and the comparison is constant time.
func (h *PasswordHasher) Verify(password, encoded string) error {
	params, salt, want, err := decodeHash(encoded)
	if err != nil {
		return err
	}

	got := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}
	return nil
}

// decodeHash parses a PHC argon2id hash into its parameters, salt and key.
func decodeHash(encoded string) (Argon2idParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
	if len(parts) != 6 || parts[0] != "" {
		return Argon2idParams{}, nil, nil, fmt.Errorf("%w: not a PHC argon2id string", ErrMalformedHash)
	}
	if parts[1] != argon2idAlgorithm {
		return Argon2idParams{}, nil, nil, fmt.Errorf("%w: %q", ErrUnsupportedHash, parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Argon2idParams{}, nil, nil, fmt.Errorf("%w: unsupported version %q", ErrMalformedHash, parts[2])
	}

	var params Argon2idParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.MemoryKiB, &params.Iterations, &params.Parallelism); err != nil {
		return Argon2idParams{}, nil, nil, fmt.Errorf("%w: unreadable cost %q", ErrMalformedHash, parts[3])
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2idParams{}, nil, nil, fmt.Errorf("%w: salt is not base64: %v", ErrMalformedHash, err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2idParams{}, nil, nil, fmt.Errorf("%w: key is not base64: %v", ErrMalformedHash, err)
	}

	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	if err := validateParams(params); err != nil {
		return Argon2idParams{}, nil, nil, err
	}

	return params, salt, key, nil
}

// validateParams is shared by minting and parsing: the same bounds that keep a
// misconfigured hasher from starting keep a tampered row from allocating without
// limit.
func validateParams(params Argon2idParams) error {
	switch {
	case params.MemoryKiB < minMemoryKiB || params.MemoryKiB > maxMemoryKiB:
		return fmt.Errorf("authn: memory cost %d KiB is outside [%d, %d]", params.MemoryKiB, minMemoryKiB, maxMemoryKiB)
	case params.Iterations < 1 || params.Iterations > maxIterations:
		return fmt.Errorf("authn: iteration count %d is outside [1, %d]", params.Iterations, maxIterations)
	case params.Parallelism < 1 || params.Parallelism > maxParallelism:
		return fmt.Errorf("authn: parallelism %d is outside [1, %d]", params.Parallelism, maxParallelism)
	case params.SaltLength < minSaltLength || params.SaltLength > maxSaltLength:
		return fmt.Errorf("authn: salt length %d is outside [%d, %d]", params.SaltLength, minSaltLength, maxSaltLength)
	case params.KeyLength < minKeyLength || params.KeyLength > maxKeyLength:
		return fmt.Errorf("authn: key length %d is outside [%d, %d]", params.KeyLength, minKeyLength, maxKeyLength)
	}
	return nil
}

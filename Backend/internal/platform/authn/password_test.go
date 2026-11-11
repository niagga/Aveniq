package authn_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// cheapParams keeps the tests fast. The production costs live in
// DefaultArgon2idParams and are exercised only by the shape assertions.
func cheapParams() authn.Argon2idParams {
	return authn.Argon2idParams{
		MemoryKiB:   8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func TestDefaultParamsMatchTheAgreedCost(t *testing.T) {
	params := authn.DefaultArgon2idParams()

	// Decision 6 of L11: m=64 MiB, t=3, p=4, 16-byte salt, 32-byte key.
	if params.MemoryKiB != 64*1024 || params.Iterations != 3 || params.Parallelism != 4 {
		t.Errorf("cost = m=%d t=%d p=%d, want m=65536 t=3 p=4", params.MemoryKiB, params.Iterations, params.Parallelism)
	}
	if params.SaltLength != 16 || params.KeyLength != 32 {
		t.Errorf("salt/key = %d/%d, want 16/32", params.SaltLength, params.KeyLength)
	}
}

func TestPasswordHasherRoundTrip(t *testing.T) {
	hasher, err := authn.NewPasswordHasher(cheapParams())
	if err != nil {
		t.Fatalf("cannot build the hasher: %v", err)
	}

	encoded, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("cannot hash: %v", err)
	}

	// The stored value must carry its own parameters: if the cost is ever raised,
	// old hashes still have to verify, so nothing may depend on a constant.
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("encoded hash = %q, want the PHC form with the configured parameters", encoded)
	}

	if err := hasher.Verify("correct horse battery staple", encoded); err != nil {
		t.Fatalf("a correct password must verify: %v", err)
	}
	if err := hasher.Verify("wrong horse battery staple", encoded); !errors.Is(err, authn.ErrPasswordMismatch) {
		t.Fatalf("error = %v, want %v", err, authn.ErrPasswordMismatch)
	}
}

func TestPasswordHasherSaltsEveryHash(t *testing.T) {
	hasher, err := authn.NewPasswordHasher(cheapParams())
	if err != nil {
		t.Fatalf("cannot build the hasher: %v", err)
	}

	first, err := hasher.Hash("same password")
	if err != nil {
		t.Fatalf("cannot hash: %v", err)
	}
	second, err := hasher.Hash("same password")
	if err != nil {
		t.Fatalf("cannot hash: %v", err)
	}

	if first == second {
		t.Fatal("two hashes of the same password are identical: the salt is not random")
	}
	// A deterministic salt would also make the second hash verify against the
	// first, which is what a rainbow table needs.
	if err := hasher.Verify("same password", first); err != nil {
		t.Fatalf("the first hash does not verify: %v", err)
	}
	if err := hasher.Verify("same password", second); err != nil {
		t.Fatalf("the second hash does not verify: %v", err)
	}
}

// TestPasswordVerifyReadsParametersFromTheStoredHash is the regression guard for
// decision 6: a hash minted with cheaper costs must still verify against a hasher
// configured with the production ones. If the verifier ever reads its costs from
// a constant instead of the stored string, raising them locks every existing user
// out of the product and this test fails first.
func TestPasswordVerifyReadsParametersFromTheStoredHash(t *testing.T) {
	cheap, err := authn.NewPasswordHasher(cheapParams())
	if err != nil {
		t.Fatalf("cannot build the cheap hasher: %v", err)
	}

	encoded, err := cheap.Hash("legacy password")
	if err != nil {
		t.Fatalf("cannot hash: %v", err)
	}

	// A hasher with different (and, in production, higher) costs.
	other := cheapParams()
	other.MemoryKiB = 16 * 1024
	other.Iterations = 2
	upgraded, err := authn.NewPasswordHasher(other)
	if err != nil {
		t.Fatalf("cannot build the upgraded hasher: %v", err)
	}

	if err := upgraded.Verify("legacy password", encoded); err != nil {
		t.Fatalf("a hash minted with other costs must still verify: %v", err)
	}
	if err := upgraded.Verify("wrong", encoded); !errors.Is(err, authn.ErrPasswordMismatch) {
		t.Fatalf("error = %v, want %v", err, authn.ErrPasswordMismatch)
	}
}

func TestPasswordHasherRejectsMalformedStoredHashes(t *testing.T) {
	hasher, err := authn.NewPasswordHasher(cheapParams())
	if err != nil {
		t.Fatalf("cannot build the hasher: %v", err)
	}

	for _, encoded := range []string{
		"",
		"plain-text-password",
		"$argon2id$",
		"$argon2id$v=19$m=8192,t=1,p=1$onlyfourparts",
		"$argon2id$v=19$m=8192,t=1$c2FsdHNhbHRzYWx0$a2V5",
		"$argon2id$v=19$m=abc,t=1,p=1$c2FsdHNhbHRzYWx0$a2V5",
		"$argon2id$v=19$m=8192,t=1,p=1$not base64!$a2V5",
		"$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0$a2V5",
	} {
		err := hasher.Verify("whatever", encoded)
		if err == nil {
			t.Fatalf("Verify(%q) = nil, want a malformed-hash error", encoded)
		}
		if errors.Is(err, authn.ErrPasswordMismatch) {
			t.Fatalf("Verify(%q) reported a password mismatch: a corrupt hash is not a wrong password", encoded)
		}
	}
}

func TestPasswordHasherRejectsOtherAlgorithms(t *testing.T) {
	hasher, err := authn.NewPasswordHasher(cheapParams())
	if err != nil {
		t.Fatalf("cannot build the hasher: %v", err)
	}

	err = hasher.Verify("x", "$argon2i$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5")
	if !errors.Is(err, authn.ErrUnsupportedHash) {
		t.Fatalf("error = %v, want %v", err, authn.ErrUnsupportedHash)
	}
}

func TestPasswordHashRejectsEmptyPassword(t *testing.T) {
	hasher, err := authn.NewPasswordHasher(cheapParams())
	if err != nil {
		t.Fatalf("cannot build the hasher: %v", err)
	}

	if _, err := hasher.Hash(""); !errors.Is(err, authn.ErrEmptyPassword) {
		t.Fatalf("error = %v, want %v", err, authn.ErrEmptyPassword)
	}
}

func TestNewPasswordHasherRejectsUnusableParameters(t *testing.T) {
	for name, mutate := range map[string]func(*authn.Argon2idParams){
		"no memory":      func(p *authn.Argon2idParams) { p.MemoryKiB = 0 },
		"no iterations":  func(p *authn.Argon2idParams) { p.Iterations = 0 },
		"no threads":     func(p *authn.Argon2idParams) { p.Parallelism = 0 },
		"short salt":     func(p *authn.Argon2idParams) { p.SaltLength = 4 },
		"short key":      func(p *authn.Argon2idParams) { p.KeyLength = 8 },
		"absurd memory":  func(p *authn.Argon2idParams) { p.MemoryKiB = 4 * 1024 * 1024 },
		"absurd threads": func(p *authn.Argon2idParams) { p.Parallelism = 255 },
	} {
		t.Run(name, func(t *testing.T) {
			params := cheapParams()
			mutate(&params)
			if _, err := authn.NewPasswordHasher(params); err == nil {
				t.Fatalf("NewPasswordHasher(%+v) = nil error, want a rejection", params)
			}
		})
	}
}

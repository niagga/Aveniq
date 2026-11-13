package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

func TestRunPrintsAHashThatVerifiesWithTheProductionParameters(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run("clave-de-prueba-larga", &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	encoded := strings.TrimSpace(out.String())
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("hash %q does not carry the production parameters", encoded)
	}

	hasher, err := authn.NewPasswordHasher(authn.DefaultArgon2idParams())
	if err != nil {
		t.Fatalf("cannot build the hasher: %v", err)
	}
	if err := hasher.Verify("clave-de-prueba-larga", encoded); err != nil {
		t.Fatalf("the printed hash does not verify its own password: %v", err)
	}
	if err := hasher.Verify("otra-clave", encoded); err == nil {
		t.Fatal("the printed hash verified a different password")
	}
}

func TestRunRefusesAnEmptyPasswordAndPrintsNothing(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run("", &out); err == nil {
		t.Fatal("expected an empty password to be refused")
	}
	if out.Len() != 0 {
		t.Fatalf("printed %q for an empty password", out.String())
	}
}

// Command passhash prints an argon2id hash of the password in AVENIQ_SEED_PASSWORD, with the
// production parameters, so the local seed (scripts/local-seed.ps1) can create accounts that
// really verify without the development password being written into the repository.
//
// The password comes from the environment, not from an argument, so it stays out of the shell
// history and out of the process list. It is development tooling: nothing in the API imports it.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

const passwordEnv = "AVENIQ_SEED_PASSWORD"

func main() {
	if err := run(os.Getenv(passwordEnv), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "passhash:", err)
		os.Exit(1)
	}
}

// run writes the encoded hash of password to out. An empty password is refused rather than
// hashed: an empty seed password would silently create accounts that anyone can enter.
func run(password string, out io.Writer) error {
	if password == "" {
		return errors.New(passwordEnv + " is not set")
	}
	hasher, err := authn.NewPasswordHasher(authn.DefaultArgon2idParams())
	if err != nil {
		return err
	}
	encoded, err := hasher.Hash(password)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, encoded)
	return err
}

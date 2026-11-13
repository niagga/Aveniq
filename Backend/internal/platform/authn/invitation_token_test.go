package authn_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

func TestInvitationTokenRoundTrip(t *testing.T) {
	tenantID := uuid.NewString()

	minted, err := authn.NewInvitationToken(tenantID)
	if err != nil {
		t.Fatalf("NewInvitationToken: %v", err)
	}
	if !strings.HasPrefix(minted.Raw, "inv1."+tenantID+".") {
		t.Fatalf("token %q does not carry the inv1 prefix and the tenant hint", minted.Raw)
	}

	parsed, err := authn.ParseInvitationToken(minted.Raw)
	if err != nil {
		t.Fatalf("ParseInvitationToken of a freshly minted token: %v", err)
	}
	if parsed.TenantID != tenantID {
		t.Fatalf("tenant hint = %q, want %q", parsed.TenantID, tenantID)
	}
	if parsed.Hash != minted.Hash {
		t.Fatal("the hash derived from the presented token must match the minted one")
	}
}

func TestInvitationTokensAreUnique(t *testing.T) {
	tenantID := uuid.NewString()
	first, err := authn.NewInvitationToken(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := authn.NewInvitationToken(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Raw == second.Raw || first.Hash == second.Hash {
		t.Fatal("two invitations for the same practice must not share a secret")
	}
}

func TestInvitationTokenHashCoversTheWholeString(t *testing.T) {
	// Moving a secret to another practice changes the tenant segment, and the hash
	// must change with it: the same secret under another hint matches no row.
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	minted, err := authn.NewInvitationToken(tenantA)
	if err != nil {
		t.Fatal(err)
	}
	moved := strings.Replace(minted.Raw, tenantA, tenantB, 1)

	parsed, err := authn.ParseInvitationToken(moved)
	if err != nil {
		t.Fatalf("a well-formed token under another hint must parse: %v", err)
	}
	if parsed.Hash == minted.Hash {
		t.Fatal("the hash must cover the tenant segment")
	}
}

func TestInvitationTokenRefusesWrongShapes(t *testing.T) {
	tenantID := uuid.NewString()
	minted, err := authn.NewInvitationToken(tenantID)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Split(minted.Raw, ".")[2]

	cases := map[string]string{
		"empty":                "",
		"refresh prefix":       "v1." + tenantID + "." + secret,
		"two segments":         "inv1." + tenantID,
		"tenant is not a UUID": "inv1.not-a-uuid." + secret,
		"secret not base64":    "inv1." + tenantID + ".***",
		"secret too short":     "inv1." + tenantID + ".AAAA",
		"surrounding spaces":   " " + minted.Raw,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := authn.ParseInvitationToken(raw); !errors.Is(err, authn.ErrTokenMalformed) {
				t.Fatalf("ParseInvitationToken(%q) = %v, want ErrTokenMalformed", raw, err)
			}
		})
	}
}

func TestNewInvitationTokenRefusesANonUUIDTenant(t *testing.T) {
	if _, err := authn.NewInvitationToken("practica-a"); err == nil {
		t.Fatal("a tenant that is not a UUID must not mint a token")
	}
}

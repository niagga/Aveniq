package authn_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

func TestNewRefreshTokenShape(t *testing.T) {
	tenantID := uuid.NewString()

	token, err := authn.NewRefreshToken(tenantID)
	if err != nil {
		t.Fatalf("cannot mint a refresh token: %v", err)
	}

	// Decision 2 of L11: v1.<tenant-uuid>.<32 random bytes base64url>.
	parts := strings.Split(token.Raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token %q has %d segments, want 3", token.Raw, len(parts))
	}
	if parts[0] != "v1" {
		t.Errorf("version segment = %q, want %q", parts[0], "v1")
	}
	if parts[1] != tenantID {
		t.Errorf("tenant segment = %q, want %q", parts[1], tenantID)
	}
	if token.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", token.TenantID, tenantID)
	}
	// 32 bytes in unpadded base64url is 43 characters.
	if len(parts[2]) != 43 {
		t.Errorf("secret segment is %d characters, want 43 (32 bytes)", len(parts[2]))
	}
	// The stored value is the SHA-256 of the whole token, hex encoded.
	if len(token.Hash) != 64 {
		t.Errorf("hash length = %d, want 64 hex characters", len(token.Hash))
	}
}

func TestRefreshTokensAreUnique(t *testing.T) {
	tenantID := uuid.NewString()

	seen := make(map[string]bool)
	for i := 0; i < 64; i++ {
		token, err := authn.NewRefreshToken(tenantID)
		if err != nil {
			t.Fatalf("cannot mint a refresh token: %v", err)
		}
		if seen[token.Raw] {
			t.Fatalf("the generator repeated a token after %d attempts: the secret is not random", i)
		}
		seen[token.Raw] = true
	}
}

func TestNewRefreshTokenRejectsABadTenant(t *testing.T) {
	for _, tenantID := range []string{"", "not-a-uuid", "   "} {
		if _, err := authn.NewRefreshToken(tenantID); err == nil {
			t.Fatalf("NewRefreshToken(%q) = nil error, want a rejection", tenantID)
		}
	}
}

func TestParseRefreshTokenRoundTrip(t *testing.T) {
	tenantID := uuid.NewString()

	minted, err := authn.NewRefreshToken(tenantID)
	if err != nil {
		t.Fatalf("cannot mint a refresh token: %v", err)
	}

	parsed, err := authn.ParseRefreshToken(minted.Raw)
	if err != nil {
		t.Fatalf("cannot parse a token this package just minted: %v", err)
	}
	if parsed.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", parsed.TenantID, tenantID)
	}
	if parsed.Hash != minted.Hash {
		t.Errorf("Hash = %q, want %q: the hash must cover the whole token", parsed.Hash, minted.Hash)
	}
}

func TestParseRefreshTokenRejectsMalformed(t *testing.T) {
	tenantID := uuid.NewString()
	valid, err := authn.NewRefreshToken(tenantID)
	if err != nil {
		t.Fatalf("cannot mint a refresh token: %v", err)
	}
	secret := strings.Split(valid.Raw, ".")[2]

	for name, raw := range map[string]string{
		"empty":            "",
		"no dots":          "v1" + tenantID + secret,
		"two segments":     "v1." + tenantID,
		"four segments":    "v1." + tenantID + "." + secret + ".extra",
		"wrong version":    "v2." + tenantID + "." + secret,
		"empty version":    "." + tenantID + "." + secret,
		"tenant not uuid":  "v1.not-a-uuid." + secret,
		"short secret":     "v1." + tenantID + ".c2hvcnQ",
		"empty secret":     "v1." + tenantID + ".",
		"secret not b64":   "v1." + tenantID + "." + strings.Repeat("!", 43),
		"upper case v1":    "V1." + tenantID + "." + secret,
		"trailing newline": valid.Raw + "\n",
		"leading space":    " " + valid.Raw,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := authn.ParseRefreshToken(raw); !errors.Is(err, authn.ErrTokenMalformed) {
				t.Fatalf("ParseRefreshToken(%q) error = %v, want %v", raw, err, authn.ErrTokenMalformed)
			}
		})
	}
}

// TestRefreshHashCoversTheWholeToken fixes the property decision 2 relies on: the
// tenant segment is a lookup hint, not a permission. Because the stored hash
// covers the complete token, changing any segment stops matching any row, and
// the secret alone is never enough to be accepted.
func TestRefreshHashCoversTheWholeToken(t *testing.T) {
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()

	tokenA, err := authn.NewRefreshToken(tenantA)
	if err != nil {
		t.Fatalf("cannot mint a refresh token: %v", err)
	}
	tokenB, err := authn.NewRefreshToken(tenantB)
	if err != nil {
		t.Fatalf("cannot mint a refresh token: %v", err)
	}

	secret := strings.Split(tokenA.Raw, ".")[2]
	rehomed := "v1." + tenantB + "." + secret

	if authn.HashRefreshToken(rehomed) == tokenA.Hash {
		t.Fatal("moving the secret to another tenant produced the same hash: the tenant would be a permission")
	}
	if authn.HashRefreshToken(tokenB.Raw) == tokenA.Hash {
		t.Fatal("two different tokens produced the same hash")
	}
	if authn.HashRefreshToken(tokenA.Raw) != tokenA.Hash {
		t.Fatal("HashRefreshToken is not deterministic")
	}
}

func TestAccessTokensRoundTrip(t *testing.T) {
	now := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	tokens, err := authn.NewAccessTokens([]byte("test-signing-key-material"), 15*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatalf("cannot build the signer: %v", err)
	}

	tenantID := uuid.NewString()
	signed, err := tokens.Issue(authn.AccessClaims{
		Subject:  uuid.NewString(),
		TenantID: tenantID,
		Role:     "owner",
	})
	if err != nil {
		t.Fatalf("cannot issue: %v", err)
	}

	claims, err := tokens.Verify(signed)
	if err != nil {
		t.Fatalf("cannot verify a token this package just issued: %v", err)
	}
	if claims.TenantID != tenantID || claims.Role != "owner" {
		t.Errorf("claims = %+v, want the tenant and role that were issued", claims)
	}
	// Decision 5: 15 minutes, because E1 has no access-token revocation.
	if got := claims.ExpiresAt.Sub(claims.IssuedAt); got != 15*time.Minute {
		t.Errorf("lifetime = %v, want 15m", got)
	}
	if !claims.IssuedAt.Equal(now) {
		t.Errorf("IssuedAt = %v, want %v", claims.IssuedAt, now)
	}
}

func TestAccessTokensRejectMisuse(t *testing.T) {
	now := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	tokens, err := authn.NewAccessTokens([]byte("test-signing-key-material"), 15*time.Minute, clock)
	if err != nil {
		t.Fatalf("cannot build the signer: %v", err)
	}
	signed, err := tokens.Issue(authn.AccessClaims{Subject: uuid.NewString(), TenantID: uuid.NewString(), Role: "admin"})
	if err != nil {
		t.Fatalf("cannot issue: %v", err)
	}

	otherKey, err := authn.NewAccessTokens([]byte("a-different-signing-key"), 15*time.Minute, clock)
	if err != nil {
		t.Fatalf("cannot build the second signer: %v", err)
	}

	later := func() time.Time { return now.Add(16 * time.Minute) }
	expired, err := authn.NewAccessTokens([]byte("test-signing-key-material"), 15*time.Minute, later)
	if err != nil {
		t.Fatalf("cannot build the expired-check signer: %v", err)
	}

	parts := strings.Split(signed, ".")
	tampered := parts[0] + "." + parts[1] + "x." + parts[2]
	stripped := parts[0] + "." + parts[1] + "."

	for name, tc := range map[string]struct {
		verifier *authn.AccessTokens
		token    string
		want     error
	}{
		"wrong key":      {otherKey, signed, authn.ErrTokenSignature},
		"tampered":       {tokens, tampered, authn.ErrTokenSignature},
		"signature gone": {tokens, stripped, authn.ErrTokenSignature},
		"expired":        {expired, signed, authn.ErrTokenExpired},
		"empty":          {tokens, "", authn.ErrTokenMalformed},
		"not a token":    {tokens, "hello", authn.ErrTokenMalformed},
		"two segments":   {tokens, parts[0] + "." + parts[1], authn.ErrTokenMalformed},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.verifier.Verify(tc.token); !errors.Is(err, tc.want) {
				t.Fatalf("Verify error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAccessTokensRequireCompleteClaims(t *testing.T) {
	tokens, err := authn.NewAccessTokens([]byte("test-signing-key-material"), time.Minute, time.Now)
	if err != nil {
		t.Fatalf("cannot build the signer: %v", err)
	}

	for name, claims := range map[string]authn.AccessClaims{
		"no subject":   {TenantID: uuid.NewString(), Role: "owner"},
		"no tenant":    {Subject: uuid.NewString(), Role: "owner"},
		"bad tenant":   {Subject: uuid.NewString(), TenantID: "nope", Role: "owner"},
		"no role":      {Subject: uuid.NewString(), TenantID: uuid.NewString()},
		"empty claims": {},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tokens.Issue(claims); err == nil {
				t.Fatalf("Issue(%+v) = nil error, want a rejection", claims)
			}
		})
	}
}

func TestNewAccessTokensRejectsUnusableConfiguration(t *testing.T) {
	if _, err := authn.NewAccessTokens(nil, time.Minute, time.Now); err == nil {
		t.Fatal("an empty signing key must be rejected")
	}
	if _, err := authn.NewAccessTokens([]byte("short"), time.Minute, time.Now); err == nil {
		t.Fatal("a short signing key must be rejected")
	}
	if _, err := authn.NewAccessTokens([]byte("test-signing-key-material"), 0, time.Now); err == nil {
		t.Fatal("a zero lifetime must be rejected")
	}
	if _, err := authn.NewAccessTokens([]byte("test-signing-key-material"), time.Minute, nil); err == nil {
		t.Fatal("a nil clock must be rejected")
	}
}

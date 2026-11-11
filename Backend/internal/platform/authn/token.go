package authn

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors for both token formats.
var (
	// ErrTokenMalformed reports a token that is not even shaped like one.
	ErrTokenMalformed = errors.New("authn: malformed token")
	// ErrTokenSignature reports a token whose signature does not verify. It
	// covers tampering, a foreign signer and a truncated signature.
	ErrTokenSignature = errors.New("authn: token signature does not verify")
	// ErrTokenExpired reports a correctly signed token past its expiry.
	ErrTokenExpired = errors.New("authn: token expired")
)

const (
	// refreshTokenVersion prefixes the refresh format. Changing it changes every
	// stored hash, which is the point: a replayed old-format token matches no row.
	refreshTokenVersion = "v1"
	// accessTokenVersion does the same for access tokens.
	accessTokenVersion = "v1"
	// refreshSecretBytes is the entropy of the secret segment: 32 bytes.
	refreshSecretBytes = 32
	// minSigningKeyBytes is the shortest HMAC key accepted. A short key is a
	// configuration mistake, and it fails at startup, not on a request.
	minSigningKeyBytes = 16
)

// RefreshToken is one issued refresh grant.
//
// Raw is the only value that goes to the client. Hash is what the database
// stores: the SHA-256 of the complete token, so that moving the secret to
// another tenant changes the hash and matches nothing. The tenant inside the
// token is a lookup hint — to know which scope to open before RLS can be asked
// anything — never a permission (decision 2 of L11).
type RefreshToken struct {
	Raw      string
	TenantID string
	Hash     string
}

// NewRefreshToken mints a token for a tenant: v1.<tenant-uuid>.<43 base64url>.
func NewRefreshToken(tenantID string) (RefreshToken, error) {
	if _, err := uuid.Parse(tenantID); err != nil {
		return RefreshToken{}, fmt.Errorf("authn: %q is not a valid tenant UUID: %w", tenantID, err)
	}

	secret := make([]byte, refreshSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return RefreshToken{}, fmt.Errorf("authn: cannot read random secret: %w", err)
	}

	raw := refreshTokenVersion + "." + tenantID + "." + base64.RawURLEncoding.EncodeToString(secret)
	return RefreshToken{Raw: raw, TenantID: tenantID, Hash: HashRefreshToken(raw)}, nil
}

// ParseRefreshToken validates a presented token and derives the values a
// rotation needs. It checks shape only: whether the hash exists, whether the row
// is revoked and whether the chain was reused are storage questions.
func ParseRefreshToken(raw string) (RefreshToken, error) {
	if raw != strings.TrimSpace(raw) {
		return RefreshToken{}, fmt.Errorf("%w: surrounding whitespace", ErrTokenMalformed)
	}

	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return RefreshToken{}, fmt.Errorf("%w: want 3 segments, got %d", ErrTokenMalformed, len(parts))
	}
	if parts[0] != refreshTokenVersion {
		return RefreshToken{}, fmt.Errorf("%w: unsupported version %q", ErrTokenMalformed, parts[0])
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return RefreshToken{}, fmt.Errorf("%w: tenant segment is not a UUID", ErrTokenMalformed)
	}

	secret, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return RefreshToken{}, fmt.Errorf("%w: secret is not base64url: %v", ErrTokenMalformed, err)
	}
	if len(secret) != refreshSecretBytes {
		return RefreshToken{}, fmt.Errorf("%w: secret is %d bytes, want %d", ErrTokenMalformed, len(secret), refreshSecretBytes)
	}

	return RefreshToken{Raw: raw, TenantID: parts[1], Hash: HashRefreshToken(raw)}, nil
}

// HashRefreshToken returns the stored form of a token: hex-encoded SHA-256 over
// the complete string, prefix and tenant included.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// AccessClaims is the identity a verified access token carries. The field names
// are the claims; there is no registered-algorithm field, because this package
// only ever verifies its own HMAC-SHA256 signature (decision 5 of L11).
type AccessClaims struct {
	Subject   string    `json:"sub"`
	TenantID  string    `json:"tid"`
	Role      string    `json:"role"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

// AccessTokens signs and verifies short-lived access tokens with HMAC-SHA256.
//
// A signed JWT library was deliberately not adopted: an `alg` field that arrives
// with the token is a trust decision, and the algorithm-confusion family exists
// because verifiers honour it. Here there is nothing to negotiate.
type AccessTokens struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewAccessTokens builds the signer. The clock is injected so that expiry is
// testable without sleeping.
func NewAccessTokens(secret []byte, ttl time.Duration, now func() time.Time) (*AccessTokens, error) {
	if len(secret) < minSigningKeyBytes {
		return nil, fmt.Errorf("authn: signing key is %d bytes, want at least %d", len(secret), minSigningKeyBytes)
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("authn: token lifetime must be positive, got %s", ttl)
	}
	if now == nil {
		return nil, errors.New("authn: token clock must not be nil")
	}

	return &AccessTokens{
		secret: append([]byte(nil), secret...),
		ttl:    ttl,
		now:    now,
	}, nil
}

// TTL is the configured access-token lifetime, so the API can report it.
func (a *AccessTokens) TTL() time.Duration { return a.ttl }

// Issue signs the claims: v1.<base64url(json claims)>.<base64url(hmac)>.
func (a *AccessTokens) Issue(claims AccessClaims) (string, error) {
	if claims.Subject == "" {
		return "", errors.New("authn: access claims need a subject")
	}
	if _, err := uuid.Parse(claims.TenantID); err != nil {
		return "", fmt.Errorf("authn: access claims need a tenant UUID: %w", err)
	}
	if claims.Role == "" {
		return "", errors.New("authn: access claims need a role")
	}

	if claims.IssuedAt.IsZero() {
		claims.IssuedAt = a.now().UTC()
	}
	claims.ExpiresAt = claims.IssuedAt.Add(a.ttl)

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("authn: cannot encode the claims: %w", err)
	}

	signingInput := accessTokenVersion + "." + base64.RawURLEncoding.EncodeToString(payload)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(a.sign(signingInput)), nil
}

// Verify checks the signature before the expiry, and never trusts a field of the
// payload to describe how it was signed.
func (a *AccessTokens) Verify(token string) (AccessClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != accessTokenVersion {
		return AccessClaims{}, fmt.Errorf("%w: want %s.<payload>.<signature>", ErrTokenMalformed, accessTokenVersion)
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessClaims{}, fmt.Errorf("%w: payload is not base64url: %v", ErrTokenMalformed, err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return AccessClaims{}, fmt.Errorf("%w: signature is not base64url: %v", ErrTokenMalformed, err)
	}

	want := a.sign(parts[0] + "." + parts[1])
	if subtle.ConstantTimeCompare(want, signature) != 1 {
		return AccessClaims{}, ErrTokenSignature
	}

	var claims AccessClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return AccessClaims{}, fmt.Errorf("%w: payload is not JSON: %v", ErrTokenMalformed, err)
	}
	if claims.Subject == "" || claims.TenantID == "" || claims.Role == "" {
		return AccessClaims{}, fmt.Errorf("%w: payload is missing a required claim", ErrTokenMalformed)
	}

	if !a.now().Before(claims.ExpiresAt) {
		return AccessClaims{}, ErrTokenExpired
	}
	return claims, nil
}

// sign computes the HMAC over the versioned signing input.
func (a *AccessTokens) sign(signingInput string) []byte {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}

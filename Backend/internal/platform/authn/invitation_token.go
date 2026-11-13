package authn

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// invitationTokenVersion prefixes the invitation format. It differs from the
// refresh prefix on purpose: a refresh grant presented as an invitation must
// match no row, and the other way around.
const invitationTokenVersion = "inv1"

// InvitationToken is the link secret an owner shares with an invitee.
//
// Raw goes to the invitee once and is never stored. Hash is what the database
// keeps: SHA-256 over the complete string, tenant segment included, so a secret
// moved to another practice matches nothing (the same rule as refresh grants).
// The tenant inside the token is a lookup hint, needed to open the right
// tenant scope before row-level security can be asked anything, and never a
// permission.
type InvitationToken struct {
	Raw      string
	TenantID string
	Hash     string
}

// NewInvitationToken mints a token for a practice: inv1.<tenant-uuid>.<43 base64url>.
func NewInvitationToken(tenantID string) (InvitationToken, error) {
	if _, err := uuid.Parse(tenantID); err != nil {
		return InvitationToken{}, fmt.Errorf("authn: %q is not a valid tenant UUID: %w", tenantID, err)
	}

	secret := make([]byte, refreshSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return InvitationToken{}, fmt.Errorf("authn: cannot read random secret: %w", err)
	}

	raw := invitationTokenVersion + "." + tenantID + "." + base64.RawURLEncoding.EncodeToString(secret)
	return InvitationToken{Raw: raw, TenantID: tenantID, Hash: HashInvitationToken(raw)}, nil
}

// ParseInvitationToken checks the shape of a presented invitation token and
// derives the lookup values. Whether the hash exists and whether the invitation
// is still usable are storage and domain questions, not shape questions.
func ParseInvitationToken(raw string) (InvitationToken, error) {
	if raw != strings.TrimSpace(raw) {
		return InvitationToken{}, fmt.Errorf("%w: surrounding whitespace", ErrTokenMalformed)
	}

	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return InvitationToken{}, fmt.Errorf("%w: want 3 segments, got %d", ErrTokenMalformed, len(parts))
	}
	if parts[0] != invitationTokenVersion {
		return InvitationToken{}, fmt.Errorf("%w: unsupported version %q", ErrTokenMalformed, parts[0])
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return InvitationToken{}, fmt.Errorf("%w: tenant segment is not a UUID", ErrTokenMalformed)
	}

	secret, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return InvitationToken{}, fmt.Errorf("%w: secret is not base64url: %v", ErrTokenMalformed, err)
	}
	if len(secret) != refreshSecretBytes {
		return InvitationToken{}, fmt.Errorf("%w: secret is %d bytes, want %d", ErrTokenMalformed, len(secret), refreshSecretBytes)
	}

	return InvitationToken{Raw: raw, TenantID: parts[1], Hash: HashInvitationToken(raw)}, nil
}

// HashInvitationToken is the stored form: hex SHA-256 over the whole token.
func HashInvitationToken(raw string) string {
	return HashRefreshToken(raw)
}

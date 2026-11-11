// Package auth holds the authentication use cases: login, refresh and logout.
//
// It orchestrates the IAM domain through the port contracts and owns the
// transaction boundaries; it knows nothing about HTTP, and it never builds a
// token by hand. Decisions 1, 3 and 8 of L11 live here.
package auth

import (
	"errors"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

const (
	// RefreshSessionTTL is how long one refresh grant lives. Rotation mints a new
	// one on every use, so this bounds an abandoned session, not an active one.
	RefreshSessionTTL = 30 * 24 * time.Hour
)

// Errors the delivery layer maps onto transport codes. They are deliberately
// coarse: a caller learns "these credentials do not work", never which half was
// wrong (decision 8 of L11).
var (
	// ErrInvalidCredentials covers an unknown tenant, an unknown email, an
	// inactive user and a wrong password with one indistinguishable answer.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrInvalidRefreshToken covers a token that does not parse, one that matches
	// no session, and one whose user disappeared.
	ErrInvalidRefreshToken = errors.New("auth: invalid refresh token")
	// ErrRefreshReused reports the reuse signal: a token that had already been
	// rotated came back, so the whole chain was revoked.
	ErrRefreshReused = errors.New("auth: refresh token was already used, the session chain was revoked")
	// ErrSessionExpired reports a valid token whose grant is past its expiry.
	ErrSessionExpired = errors.New("auth: the refresh session expired")
)

// PasswordVerifier checks a plaintext password against a stored hash. The
// application depends on this narrow contract instead of the concrete hasher so
// that tests do not pay the real argon2id cost.
type PasswordVerifier interface {
	Verify(password, encoded string) error
}

// AccessTokenIssuer signs the short-lived access token and reports its lifetime.
// *authn.AccessTokens satisfies it directly.
type AccessTokenIssuer interface {
	Issue(claims authn.AccessClaims) (string, error)
	TTL() time.Duration
}

// Session is what a successful login or refresh hands to the client: the token
// pair plus the identity those tokens now carry.
type Session struct {
	AccessToken      string
	ExpiresIn        time.Duration
	RefreshToken     string
	RefreshExpiresAt time.Time
	UserID           string
	TenantID         string
	Role             string
}

// sessionIssuer is the part login and refresh share: minting a refresh grant and
// signing the access token that goes with it. Keeping it in one place is what
// makes the two paths produce the same shape.
type sessionIssuer struct {
	sessions port.SessionRepository
	tokens   AccessTokenIssuer
	clock    port.Clock
	ids      port.IDGenerator
}

// newSession builds an unsaved grant with a fresh identifier, a fresh secret and
// the domain's expiry invariant applied.
func (i sessionIssuer) newSession(user iam.User) (iam.Session, string, error) {
	now := i.clock.Now()

	refresh, err := authn.NewRefreshToken(string(user.TenantID))
	if err != nil {
		return iam.Session{}, "", err
	}

	session, err := iam.NewSession(
		iam.SessionID(i.ids.NewID()),
		user.TenantID,
		user.ID,
		iam.RefreshHash(refresh.Hash),
		now,
		now.Add(RefreshSessionTTL),
	)
	if err != nil {
		return iam.Session{}, "", err
	}
	return session, refresh.Raw, nil
}

// pair signs the access token and assembles the answer.
func (i sessionIssuer) pair(user iam.User, refreshToken string, refreshExpiresAt time.Time) (Session, error) {
	access, err := i.tokens.Issue(authn.AccessClaims{
		Subject:  string(user.ID),
		TenantID: string(user.TenantID),
		Role:     string(user.Role),
	})
	if err != nil {
		return Session{}, err
	}

	return Session{
		AccessToken:      access,
		ExpiresIn:        i.tokens.TTL(),
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		UserID:           string(user.ID),
		TenantID:         string(user.TenantID),
		Role:             string(user.Role),
	}, nil
}

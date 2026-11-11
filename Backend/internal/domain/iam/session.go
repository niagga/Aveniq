package iam

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// SessionID is the opaque identity of a refresh session.
type SessionID string

// RefreshHash is the opaque SHA-256 of a refresh token. The token itself is
// never stored (database comment on sessions.refresh_hash); generating tokens
// and hashes belongs to the auth layer (T14), not the domain.
type RefreshHash string

// NewRefreshHash validates a hash, mirroring sessions_refresh_hash_not_blank.
func NewRefreshHash(raw string) (RefreshHash, error) {
	if strings.Trim(raw, " ") == "" {
		return "", &ValidationError{Field: "refresh_hash", Message: "must not be blank"}
	}
	return RefreshHash(raw), nil
}

// Session is one live refresh-token grant: the record a rotation revokes and
// the reuse-detection signal points at when a token is replayed.
type Session struct {
	ID          SessionID
	TenantID    tenant.ID
	UserID      UserID
	RefreshHash RefreshHash
	UserAgent   *string
	IP          *netip.Addr
	CreatedAt   time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	ReplacedBy  *SessionID
}

// SessionOption is optional metadata for New. Optional fields stay out of the
// positional signature so the required ones remain readable.
type SessionOption func(*Session)

// WithUserAgent records the client that created the session.
func WithUserAgent(agent *string) SessionOption {
	return func(s *Session) { s.UserAgent = agent }
}

// WithIP records the source address. The domain keeps it opaque as netip.Addr;
// the database column is inet.
func WithIP(ip *string) SessionOption {
	return func(s *Session) {
		if ip == nil {
			return
		}
		if addr, err := netip.ParseAddr(*ip); err == nil {
			s.IP = &addr
		}
	}
}

// New builds a brand-new session. The expiry invariant mirrors the database
// constraint sessions_expiry_after_creation: expires_at > created_at.
func NewSession(
	id SessionID,
	tenantID tenant.ID,
	userID UserID,
	hash RefreshHash,
	createdAt, expiresAt time.Time,
	opts ...SessionOption,
) (Session, error) {
	if id == "" {
		return Session{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Session{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if userID == "" {
		return Session{}, &ValidationError{Field: "user_id", Message: "must not be empty"}
	}

	if _, err := NewRefreshHash(string(hash)); err != nil {
		return Session{}, err
	}

	if !expiresAt.After(createdAt) {
		return Session{}, fmt.Errorf("%w: expiry must be after creation", ErrInvalidTransition)
	}

	session := Session{
		ID:          id,
		TenantID:    tenantID,
		UserID:      userID,
		RefreshHash: hash,
		CreatedAt:   createdAt.UTC(),
		ExpiresAt:   expiresAt.UTC(),
	}
	for _, opt := range opts {
		opt(&session)
	}
	return session, nil
}

// Reconstitute rebuilds a session from persisted state. Stored rows already
// passed New at creation; revalidation would only duplicate the checks, so this
// constructor trusts persistence the way tenant.Reconstitute trusts canonical
// slugs — the adapter wraps any anomaly in its own error.
func ReconstituteSession(
	id SessionID,
	tenantID tenant.ID,
	userID UserID,
	hash RefreshHash,
	userAgent *string,
	ip *netip.Addr,
	createdAt, expiresAt time.Time,
	revokedAt *time.Time,
	replacedBy *SessionID,
) (Session, error) {
	if id == "" {
		return Session{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if _, err := NewRefreshHash(string(hash)); err != nil {
		return Session{}, err
	}

	return Session{
		ID:          id,
		TenantID:    tenantID,
		UserID:      userID,
		RefreshHash: hash,
		UserAgent:   userAgent,
		IP:          ip,
		CreatedAt:   createdAt.UTC(),
		ExpiresAt:   expiresAt.UTC(),
		RevokedAt:   revokedAt,
		ReplacedBy:  replacedBy,
	}, nil
}

// Revoke marks the session dead. Value receiver, mirroring the other
// aggregates: the returned Session is the new state.
func (s Session) Revoke(now time.Time) (Session, error) {
	if s.RevokedAt != nil {
		return Session{}, fmt.Errorf("%w: session is already revoked", ErrInvalidTransition)
	}
	revoked := now.UTC()
	s.RevokedAt = &revoked
	return s, nil
}

// ReplaceWith rotates the session onto its successor: revoke plus a pointer to
// the new session. Presenting this token afterwards is a reuse signal (T14).
func (s Session) ReplaceWith(successor SessionID, now time.Time) (Session, error) {
	if s.RevokedAt != nil {
		return Session{}, fmt.Errorf("%w: session is already revoked", ErrInvalidTransition)
	}
	if successor == "" || successor == s.ID {
		return Session{}, fmt.Errorf("%w: successor must be a different, existing session", ErrInvalidTransition)
	}
	revoked := now.UTC()
	s.RevokedAt = &revoked
	s.ReplacedBy = &successor
	return s, nil
}

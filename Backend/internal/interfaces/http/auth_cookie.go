package http

import (
	"net/http"
	"time"
)

// RefreshCookieConfig describes how the refresh grant is carried.
//
// The token lives in a cookie instead of the response body because the client is
// a browser: the refresh grant is the long-lived half of the pair (30 days), and
// a credential of that life in JavaScript-reachable storage is one XSS away from
// being exfiltrated. The access token stays in the body, where it is useful and
// short-lived.
type RefreshCookieConfig struct {
	Name string
	Path string
	// Secure must be true anywhere the API is not reached over plain HTTP on
	// loopback: a session cookie without TLS protection is readable on the wire.
	Secure bool
	TTL    time.Duration
}

// DefaultRefreshCookieConfig builds the cookie for this API. The path is scoped
// to the auth endpoints so that no other request carries the credential.
func DefaultRefreshCookieConfig(ttl time.Duration, secure bool) RefreshCookieConfig {
	return RefreshCookieConfig{
		Name:   "aveniq_refresh",
		Path:   APIPrefix + "/auth",
		Secure: secure,
		TTL:    ttl,
	}
}

// issue returns the Set-Cookie value for a freshly minted token.
//
// SameSite=Lax is what keeps the cookie out of cross-site POSTs, which is the
// CSRF exposure this design accepts while the frontend is same-origin. HttpOnly
// is not optional: it is the reason the token moved out of the body at all.
func (c RefreshCookieConfig) issue(raw string) string {
	return (&http.Cookie{
		Name:     c.Name,
		Value:    raw,
		Path:     c.Path,
		MaxAge:   int(c.TTL / time.Second),
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteLaxMode,
	}).String()
}

// clear returns the Set-Cookie value that removes the cookie. MaxAge=-1 is the
// instruction browsers honour; an empty value alone leaves the cookie present.
func (c RefreshCookieConfig) clear() string {
	return (&http.Cookie{
		Name:     c.Name,
		Value:    "",
		Path:     c.Path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.Secure,
		SameSite: http.SameSiteLaxMode,
	}).String()
}

// presented is the token to act on: the cookie for a browser, the body for a
// non-browser client. Accepting both keeps the API usable from a test or a CLI
// without making the browser carry the token in JavaScript.
func (c RefreshCookieConfig) presented(cookie, body string) string {
	if cookie != "" {
		return cookie
	}
	return body
}

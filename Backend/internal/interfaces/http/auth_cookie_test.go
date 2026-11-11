package http

import (
	"strings"
	"testing"
	"time"
)

func TestRefreshCookieIssue(t *testing.T) {
	config := DefaultRefreshCookieConfig(30*24*time.Hour, true)
	header := config.issue("v1.tenant.secret")

	// Every attribute here is a security decision, so each one is asserted rather
	// than inferred from the fact that it renders.
	for _, want := range []string{
		"aveniq_refresh=v1.tenant.secret",
		"Path=/api/v1/auth",
		"Max-Age=2592000",
		"HttpOnly",
		"Secure",
		"SameSite=Lax",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("Set-Cookie %q is missing %q", header, want)
		}
	}
	// The path is scoped so that no other request carries the credential.
	if strings.Contains(header, "Path=/;") || strings.Contains(header, "Path=/ ") {
		t.Errorf("the cookie escaped the auth path: %q", header)
	}
}

func TestRefreshCookieClear(t *testing.T) {
	config := DefaultRefreshCookieConfig(time.Hour, false)
	header := config.clear()

	if !strings.Contains(header, "aveniq_refresh=") {
		t.Errorf("Set-Cookie %q does not name the cookie", header)
	}
	// An empty value alone leaves the cookie in place. Go serialises a negative
	// MaxAge as "Max-Age=0", and RFC 6265 treats zero as "expire immediately",
	// which is the deletion instruction the browser actually honours.
	if !strings.Contains(header, "Max-Age=0") {
		t.Errorf("Set-Cookie %q does not expire the cookie", header)
	}
	if strings.Contains(header, "Secure") {
		t.Errorf("Set-Cookie %q marks the cookie Secure over plain HTTP: the browser would drop the deletion", header)
	}
}
func TestRefreshCookiePreferredToken(t *testing.T) {
	config := DefaultRefreshCookieConfig(time.Hour, false)

	for name, tc := range map[string]struct {
		cookie string
		body   string
		want   string
	}{
		"the cookie wins":        {"from-cookie", "from-body", "from-cookie"},
		"the body is a fallback": {"", "from-body", "from-body"},
		"neither":                {"", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := config.presented(tc.cookie, tc.body); got != tc.want {
				t.Fatalf("presented() = %q, want %q", got, tc.want)
			}
		})
	}
}

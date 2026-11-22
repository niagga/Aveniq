package landing

import (
	"net"
	"regexp"
	"strings"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// hostPattern is a DNS host name: labels of letters, digits and inner hyphens, joined by dots.
// A single label is allowed here (localhost); the table that stores custom domains demands a dot.
var hostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

const maxHostLength = 253

// NormalizeHost reduces a Host header or a query value to the form the lookup uses: lowercase,
// without a port and without a trailing dot. It refuses what is not a host name, including an
// IPv6 literal, which no practice can own.
func NormalizeHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" {
		return "", &ValidationError{Field: "host", Message: "must not be empty"}
	}
	if strings.Contains(host, "[") {
		return "", &ValidationError{Field: "host", Message: "must be a host name, not an address"}
	}
	if strings.Contains(host, ":") {
		name, _, err := net.SplitHostPort(host)
		if err != nil {
			return "", &ValidationError{Field: "host", Message: "must be a host name"}
		}
		host = name
	}
	host = strings.TrimSuffix(host, ".")

	if len(host) > maxHostLength || !hostPattern.MatchString(host) {
		return "", &ValidationError{Field: "host", Message: "must be a host name"}
	}
	return host, nil
}

// SubdomainOf returns the practice slug when host is exactly one label under base: sur.aveniq.app
// under aveniq.app is "sur". It is false for the base itself, for deeper names, for other
// domains, and for a label that is not a valid slug. host and base are compared lowercased.
func SubdomainOf(host, base string) (string, bool) {
	host = strings.ToLower(host)
	base = strings.ToLower(strings.TrimSuffix(base, "."))
	if base == "" {
		return "", false
	}

	label, ok := strings.CutSuffix(host, "."+base)
	if !ok || label == "" || strings.Contains(label, ".") {
		return "", false
	}
	slug, err := tenant.NormalizeSlug(label)
	if err != nil || slug != label {
		return "", false
	}
	return slug, true
}

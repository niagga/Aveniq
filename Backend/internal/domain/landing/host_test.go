package landing_test

import (
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/landing"
)

func TestNormalizeHostReadsTheNameTheBrowserMeant(t *testing.T) {
	cases := map[string]string{
		"sur.aveniq.app":       "sur.aveniq.app",
		"SUR.Aveniq.App":       "sur.aveniq.app",
		"sur.aveniq.app:443":   "sur.aveniq.app",
		"  sur.aveniq.app  ":   "sur.aveniq.app",
		"sur.aveniq.app.":      "sur.aveniq.app",
		"demo.localhost:3000":  "demo.localhost",
		"localhost":            "localhost",
		"consultorio-sur.test": "consultorio-sur.test",
	}
	for raw, want := range cases {
		got, err := landing.NormalizeHost(raw)
		if err != nil || got != want {
			t.Fatalf("NormalizeHost(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
}

func TestNormalizeHostRefusesWhatIsNotAHostName(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"[::1]:8080",
		"sur..aveniq.app",
		"-sur.aveniq.app",
		"sur_norte.aveniq.app",
		"sur.aveniq.app/ruta",
		"sur.aveniq.app:notaport:1",
		"user@sur.aveniq.app",
	} {
		if _, err := landing.NormalizeHost(raw); err == nil {
			t.Fatalf("NormalizeHost(%q) accepted a value that is not a host name", raw)
		}
	}
}

func TestSubdomainOfIsOneLabelUnderTheBaseDomain(t *testing.T) {
	cases := []struct {
		host, base string
		slug       string
		ok         bool
	}{
		{"sur.aveniq.test", "aveniq.test", "sur", true},
		{"demo.localhost", "localhost", "demo", true},
		{"SUR.aveniq.test", "Aveniq.Test", "sur", true},
		{"aveniq.test", "aveniq.test", "", false},
		{"a.b.aveniq.test", "aveniq.test", "", false},
		{"sur.otro.test", "aveniq.test", "", false},
		{"sur.aveniq.test.evil", "aveniq.test", "", false},
		{"-sur.aveniq.test", "aveniq.test", "", false},
		{"sur.aveniq.test", "", "", false},
		{"consultorio-sur.test", "aveniq.test", "", false},
	}
	for _, tc := range cases {
		slug, ok := landing.SubdomainOf(tc.host, tc.base)
		if ok != tc.ok || slug != tc.slug {
			t.Fatalf("SubdomainOf(%q, %q) = %q, %v; want %q, %v", tc.host, tc.base, slug, ok, tc.slug, tc.ok)
		}
	}
}

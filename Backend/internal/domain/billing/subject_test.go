package billing_test

import (
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

// A document is compared without its separators and without its case, and a fragment
// too short to be a document search is not used for one.
func TestIdentifierFragment(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"digits as typed", "30111222", "30111222"},
		{"dots are dropped", "30.111.222", "30111222"},
		{"a tax id with dashes", "20-12345678-6", "20123456786"},
		{"spaces are dropped", "20 12345678", "2012345678"},
		{"letters go to upper case", "x-1234567-l", "X1234567L"},
		{"a partial fragment keeps what it has", "111.2", "1112"},
		{"two characters are too short", "30", ""},
		{"separators alone are nothing", "..-", ""},
		{"a wildcard is not a character", "%%%", ""},
		{"a wildcard between digits is dropped", "30%11", "3011"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := billing.IdentifierFragment(tc.in); got != tc.want {
				t.Fatalf("IdentifierFragment(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

package http

import "testing"

func TestBearerToken(t *testing.T) {
	for name, tc := range map[string]struct {
		header string
		want   string
		ok     bool
	}{
		"scheme and token":           {"Bearer abc.def.ghi", "abc.def.ghi", true},
		"scheme is case-insensitive": {"bearer abc", "abc", true},
		"padding is trimmed":         {"Bearer   abc   ", "abc", true},
		"no scheme":                  {"abc", "", false},
		"empty":                      {"", "", false},
		"scheme only":                {"Bearer ", "", false},
		"scheme with no token":       {"Bearer", "", false},
		"another scheme":             {"Basic dXNlcjpwYXNz", "", false},
		"prefix without space":       {"Bearerabc", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			token, ok := bearerToken(tc.header)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if token != tc.want {
				t.Fatalf("token = %q, want %q", token, tc.want)
			}
		})
	}
}

// TestIdentityIsAbsentWithoutMiddleware pins the property the handlers rely on: an
// identity can only enter the context through the unexported key this package
// writes, so a handler that runs without the guard sees nothing rather than a
// forged value.
func TestIdentityIsAbsentWithoutMiddleware(t *testing.T) {
	if _, ok := IdentityFrom(t.Context()); ok {
		t.Fatal("the plain request context already carries an identity")
	}
}

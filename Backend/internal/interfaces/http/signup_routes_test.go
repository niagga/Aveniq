package http_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// ─── doubles for the registration and the rate limit ─────────────────────────

// fakeRateLimiter counts hits per key. The window is ignored on purpose: expiry is
// the Redis adapter's job and has its own integration test, and these tests care
// about the count the middleware acts on.
type fakeRateLimiter struct {
	hits map[string]int64
	fail bool
}

func (l *fakeRateLimiter) Hit(_ context.Context, key string, _ time.Duration) (int64, error) {
	if l.fail {
		return 0, errors.New("redis: connection refused")
	}
	l.hits[key]++
	return l.hits[key], nil
}

// ─── tests ───────────────────────────────────────────────────────────────────

func signupRequest(slug, email, password string) map[string]any {
	return map[string]any{
		"practiceName": "Consultorio Sur",
		"slug":         slug,
		"ownerName":    "Ana Pérez",
		"ownerEmail":   email,
		"password":     password,
	}
}

func TestSignupCreatesAPracticeInTrialWithItsOwner(t *testing.T) {
	h := newHarness(t)

	status, body := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("consultorio-sur", "  Ana@Sur.test ", testPassword))
	if status != http.StatusCreated {
		t.Fatalf("signup = %d, want 201: %v", status, body)
	}

	practice, _ := body["tenant"].(map[string]any)
	owner, _ := body["owner"].(map[string]any)

	t.Run("the practice starts in trial, ending fourteen days after the creation", func(t *testing.T) {
		if practice["status"] != "trial" {
			t.Fatalf("status = %v, want trial", practice["status"])
		}
		end, err := time.Parse(time.RFC3339, practice["trialEndsAt"].(string))
		if err != nil {
			t.Fatalf("trialEndsAt is not RFC 3339: %v", err)
		}
		if want := testNow.Add(14 * 24 * time.Hour); !end.Equal(want) {
			t.Fatalf("trialEndsAt = %v, want %v", end, want)
		}
	})

	t.Run("the owner is the first account, canonical email, not yet verified", func(t *testing.T) {
		if owner["role"] != string(iam.RoleOwner) {
			t.Fatalf("role = %v, want owner", owner["role"])
		}
		if owner["email"] != "ana@sur.test" {
			t.Fatalf("email = %v, want the canonical form", owner["email"])
		}
		if owner["emailVerified"] != false {
			t.Fatalf("emailVerified = %v, want false until a provider exists", owner["emailVerified"])
		}
	})

	t.Run("the owner row is stored under the new practice, hashed", func(t *testing.T) {
		tenantID := practice["id"].(string)
		account, found := h.users.rows[tenantID][iam.UserID(owner["id"].(string))]
		if !found {
			t.Fatalf("owner was not stored under the new practice %s", tenantID)
		}
		if account.Role != iam.RoleOwner {
			t.Fatalf("stored role = %q, want owner", account.Role)
		}
		if string(account.PasswordHash) == testPassword {
			t.Fatal("the password was stored in clear")
		}
	})

	t.Run("the trail names the registration in the new practice", func(t *testing.T) {
		tenantID := practice["id"].(string)
		for _, event := range h.audit.events {
			if event.Action == "tenant.registered" && event.TenantID == tenantID && event.ActorID == owner["id"] {
				return
			}
		}
		t.Fatal("trail has no tenant.registered event for the new practice, acted by its owner")
	})

	t.Run("the owner reads the practice through the identity-scoped route", func(t *testing.T) {
		token := h.issue(t, owner["id"].(string), practice["id"].(string), iam.RoleOwner)
		status, body := h.do(t, http.MethodGet, "/api/v1/tenants", token, nil)
		if status != http.StatusOK {
			t.Fatalf("GET /tenants = %d, want 200: %v", status, body)
		}
		items, _ := body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("items = %d, want 1", len(items))
		}
		if first, _ := items[0].(map[string]any); first["trialEndsAt"] == nil {
			t.Fatal("the practice read back carries no trialEndsAt")
		}
	})
}

func TestSignupRefusals(t *testing.T) {
	t.Run("a slug already taken is 409", func(t *testing.T) {
		h := newHarness(t)
		if status, body := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("consultorio-sur", "ana@sur.test", testPassword)); status != http.StatusCreated {
			t.Fatalf("first signup = %d: %v", status, body)
		}
		if status, body := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("consultorio-sur", "otra@sur.test", testPassword)); status != http.StatusConflict {
			t.Fatalf("second signup with the same slug = %d, want 409: %v", status, body)
		}
	})

	t.Run("a password under twelve characters is 422 and creates nothing", func(t *testing.T) {
		h := newHarness(t)
		if status, _ := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("consultorio-sur", "ana@sur.test", "clave-corta")); status != http.StatusUnprocessableEntity {
			t.Fatalf("short password = %d, want 422", status)
		}
		if len(h.users.rows) != 0 {
			t.Fatalf("a refused signup left accounts behind: %v", h.users.rows)
		}
		if len(h.tenants.byID) != 2 {
			t.Fatalf("a refused signup left a practice behind: %d practices, want the 2 seeded", len(h.tenants.byID))
		}
	})

	t.Run("a slug outside the DNS label rule is 422", func(t *testing.T) {
		h := newHarness(t)
		if status, _ := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("Consultorio_Sur", "ana@sur.test", testPassword)); status != http.StatusUnprocessableEntity {
			t.Fatalf("invalid slug = %d, want 422", status)
		}
	})

	t.Run("a malformed owner email is 422", func(t *testing.T) {
		h := newHarness(t)
		if status, _ := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("consultorio-sur", "no-es-correo", testPassword)); status != http.StatusUnprocessableEntity {
			t.Fatalf("malformed email = %d, want 422", status)
		}
	})
}

func TestSignupIsRateLimitedPerClient(t *testing.T) {
	h := newHarness(t)

	for i, slug := range []string{"sur-uno", "sur-dos", "sur-tres", "sur-cuatro", "sur-cinco"} {
		if status, body := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest(slug, slug+"@sur.test", testPassword)); status != http.StatusCreated {
			t.Fatalf("attempt %d = %d, want 201 within the allowance: %v", i+1, status, body)
		}
	}

	if status, body := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("sur-seis", "seis@sur.test", testPassword)); status != http.StatusTooManyRequests {
		t.Fatalf("sixth attempt in the hour = %d, want 429: %v", status, body)
	}
}

func TestSignupFailsClosedWhenTheLimiterIsDown(t *testing.T) {
	h := newHarness(t)
	h.limiter.fail = true

	if status, body := h.do(t, http.MethodPost, "/api/v1/signup", "", signupRequest("consultorio-sur", "ana@sur.test", testPassword)); status != http.StatusServiceUnavailable {
		t.Fatalf("signup with the limiter down = %d, want 503: %v", status, body)
	}
	if len(h.tenants.byID) != 2 {
		t.Fatalf("a signup refused for the limiter still created a practice")
	}
}

func TestInvitationLinkIsRateLimitedPerClient(t *testing.T) {
	h := newHarness(t)

	for i := 0; i < 30; i++ {
		if status, _ := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token=no-es-un-token", "", nil); status != http.StatusGone {
			t.Fatalf("attempt %d = %d, want 410 within the allowance", i+1, status)
		}
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/invitations/preview?token=no-es-un-token", "", nil); status != http.StatusTooManyRequests {
		t.Fatalf("thirty-first attempt = %d, want 429", status)
	}
}

func TestTenantCreationIsNoLongerPublic(t *testing.T) {
	h := newHarness(t)

	status, _ := h.do(t, http.MethodPost, "/api/v1/tenants", "", map[string]any{"slug": "sin-dueno", "name": "Sin dueño"})
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /tenants = %d, want 405: a practice without an owner must not be creatable by a stranger", status)
	}
	if len(h.tenants.byID) != 2 {
		t.Fatalf("the refused request still touched the practices: %d, want the 2 seeded", len(h.tenants.byID))
	}
}

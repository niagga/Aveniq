package http_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// harnessNow is the clock every harness test runs on. Shared with newHarness so the
// expiry tests and the fixed clock cannot drift apart.
var harnessNow = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

// expiredPractice stores a practice whose trial ended a day ago, and returns its id.
// Its trial was 14 days, started 15 days before the harness clock.
func (h *harness) expiredPractice(t *testing.T) string {
	t.Helper()

	id := tenant.ID(uuid.NewString())
	created := harnessNow.Add(-15 * 24 * time.Hour)
	subject, err := tenant.Reconstitute(id, "practica-vencida-"+uuid.NewString()[:8], "Práctica vencida",
		tenant.StatusTrial, "", created, harnessNow.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("cannot build the expired practice: %v", err)
	}
	h.tenants.byID[id] = subject
	return string(id)
}

// TestAnExpiredTrialReadsButDoesNotWrite: L58. The refusal comes before validation, so a
// write that would also be invalid answers 403, not 422, which proves the guard ran first.
func TestAnExpiredTrialReadsButDoesNotWrite(t *testing.T) {
	h := newHarness(t)
	expired := h.expiredPractice(t)
	owner := h.token(t, expired, iam.RoleOwner)

	t.Run("a write is refused with trial_expired, before validation", func(t *testing.T) {
		status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, map[string]any{})
		if status != http.StatusForbidden {
			t.Fatalf("write = %d, want 403: %v", status, body)
		}
		if body["code"] != "trial_expired" {
			t.Fatalf("code = %v, want trial_expired: %v", body["code"], body)
		}
	})

	t.Run("a write that would also change state is refused", func(t *testing.T) {
		status, body := h.do(t, http.MethodPost, "/api/v1/invitations", owner, map[string]any{"role": "professional"})
		if status != http.StatusForbidden || body["code"] != "trial_expired" {
			t.Fatalf("invitation = %d %v, want 403 trial_expired", status, body)
		}
	})

	t.Run("a read still answers", func(t *testing.T) {
		status, body := h.do(t, http.MethodGet, "/api/v1/patients", owner, nil)
		if status != http.StatusOK {
			t.Fatalf("read = %d, want 200: %v", status, body)
		}
	})

	t.Run("sign-out is not refused, so the owner can leave", func(t *testing.T) {
		status, body := h.do(t, http.MethodPost, "/api/v1/auth/logout", owner, nil)
		if body["code"] == "trial_expired" {
			t.Fatalf("logout = %d refused by the trial guard: %v", status, body)
		}
	})

	t.Run("a request without a token is left to the route, which answers 401", func(t *testing.T) {
		status, _ := h.do(t, http.MethodPost, "/api/v1/patients", "", map[string]any{})
		if status != http.StatusUnauthorized {
			t.Fatalf("write without token = %d, want 401", status)
		}
	})
}

// TestAnActiveTrialWritesAsBefore is the positive control: the same write that the
// expired practice was refused reaches validation for a practice still in its trial.
func TestAnActiveTrialWritesAsBefore(t *testing.T) {
	h := newHarness(t)
	active := h.tenantID(t, "practica-a")
	owner := h.token(t, active, iam.RoleOwner)

	status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, map[string]any{})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("write in an active trial = %d, want 422 from validation: %v", status, body)
	}
}

// TestTheGuardFailsClosedWhenThePracticeCannotBeRead: a write from a practice the guard
// cannot place is refused, not let through.
func TestTheGuardFailsClosedWhenThePracticeCannotBeRead(t *testing.T) {
	h := newHarness(t)
	missing := uuid.NewString()
	owner := h.token(t, missing, iam.RoleOwner)

	status, body := h.do(t, http.MethodPost, "/api/v1/patients", owner, map[string]any{})
	if status != http.StatusServiceUnavailable || body["code"] != "trial_unavailable" {
		t.Fatalf("unknown practice write = %d %v, want 503 trial_unavailable", status, body)
	}
}

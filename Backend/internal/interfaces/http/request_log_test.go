package http_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
)

// T88 — second slice of v1 observability: the completion log line answers "to
// whom" next to "what happened". The request logger can only see the context it
// built, so the middlewares that establish the identity report it into a holder
// the logger owns. These tests drive the real router over the fakes of
// http_layer_test.go and read the log it actually produced.

// requestLogLines returns every completion log line ("http request") the router
// wrote, decoded from the JSON log the harness captures.
func requestLogLines(t *testing.T, h *harness) []map[string]any {
	t.Helper()

	decoder := json.NewDecoder(h.logs)
	var lines []map[string]any
	for {
		var line map[string]any
		if err := decoder.Decode(&line); err != nil {
			if errors.Is(err, io.EOF) {
				return lines
			}
			t.Fatalf("the log is not line-delimited JSON: %v", err)
		}
		if line["msg"] == "http request" {
			lines = append(lines, line)
		}
	}
}

// lastRequestLogLine returns the most recent completion line for a path.
func lastRequestLogLine(t *testing.T, h *harness, path string) map[string]any {
	t.Helper()

	found := false
	var line map[string]any
	for _, candidate := range requestLogLines(t, h) {
		if candidate["path"] == path {
			found = true
			line = candidate
		}
	}
	if !found {
		t.Fatalf("the log carries no completion line for %q:\n%s", path, h.logs.String())
	}
	return line
}

// requireRequestShape pins the fields the line already had before T88, so the
// identity fields are proven to be an addition, not a rewrite.
func requireRequestShape(t *testing.T, line map[string]any, status float64) map[string]any {
	t.Helper()

	if id, _ := line["request_id"].(string); id == "" {
		t.Fatalf("the completion line carries no request_id: %v", line)
	}
	if line["status"] != status {
		t.Fatalf("status = %v, want %v: %v", line["status"], status, line)
	}
	for _, field := range []string{"method", "path", "bytes", "duration"} {
		if _, present := line[field]; !present {
			t.Fatalf("the completion line lost the %q field: %v", field, line)
		}
	}
	return line
}

func requireAbsenceOfIdentity(t *testing.T, line map[string]any) {
	t.Helper()

	if _, present := line["tenant_id"]; present {
		t.Fatalf("an anonymous request logged a tenant_id: %v", line)
	}
	if _, present := line["user_id"]; present {
		t.Fatalf("an anonymous request logged a user_id: %v", line)
	}
}

// TestRequestLogCarriesTheTokenIdentity pins the purpose of the holder: a
// request whose token is accepted names the practice and the user the token
// carried, with no extra probe into the storage the handler used.
func TestRequestLogCarriesTheTokenIdentity(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	userID := uuid.NewString()
	token, err := h.tokens.Issue(authn.AccessClaims{Subject: userID, TenantID: tenantA, Role: string(iam.RoleOwner)})
	if err != nil {
		t.Fatalf("cannot issue the token: %v", err)
	}

	status, _ := h.do(t, http.MethodGet, "/api/v1/patients", token, nil)
	if status != http.StatusOK {
		t.Fatalf("list = %d, want 200", status)
	}

	line := lastRequestLogLine(t, h, "/api/v1/patients")
	requireRequestShape(t, line, 200)
	if line["tenant_id"] != tenantA {
		t.Fatalf("tenant_id = %v, want %s: the completion line must name the practice the token carried", line["tenant_id"], tenantA)
	}
	if line["user_id"] != userID {
		t.Fatalf("user_id = %v, want %s: the completion line must name the user the token carried", line["user_id"], userID)
	}
}

// TestRequestLogNamesNoIdentityOnPublicRoutes keeps the absence readable: a
// request that never authenticates logs no identity field at all, and certainly
// not an empty string that would look like a practice with no id.
func TestRequestLogNamesNoIdentityOnPublicRoutes(t *testing.T) {
	h := newHarness(t)

	status, _ := h.do(t, http.MethodGet, "/healthz", "", nil)
	if status != http.StatusOK {
		t.Fatalf("/healthz = %d, want 200", status)
	}
	requireAbsenceOfIdentity(t, requireRequestShape(t, lastRequestLogLine(t, h, "/healthz"), 200))

	// An unauthenticated attempt is anonymous too: the refusal says who was not
	// there, and inventing a tenant from the route would be a lie.
	h.logs.Reset()
	status, _ = h.do(t, http.MethodGet, "/api/v1/tenants", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("tenants without a token = %d, want 401", status)
	}
	requireAbsenceOfIdentity(t, requireRequestShape(t, lastRequestLogLine(t, h, "/api/v1/tenants"), 401))
}

// TestRequestLogsStayApartAcrossPractices pins the per-request holder: two
// consecutive requests from two practices each log only the identity its own
// token established, and neither line can name the other's tenant or user.
func TestRequestLogsStayApartAcrossPractices(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	tenantB := h.tenantID(t, "practica-b")
	userA, userB := uuid.NewString(), uuid.NewString()
	tokenA := mustIssueToken(t, h, userA, tenantA, iam.RoleOwner)
	tokenB := mustIssueToken(t, h, userB, tenantB, iam.RoleOwner)

	if status, _ := h.do(t, http.MethodGet, "/api/v1/auth/me", tokenA, nil); status != http.StatusOK {
		t.Fatalf("me = %d, want 200", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/auth/me", tokenB, nil); status != http.StatusOK {
		t.Fatalf("me = %d, want 200", status)
	}

	var mine []map[string]any
	for _, line := range requestLogLines(t, h) {
		if line["path"] == "/api/v1/auth/me" {
			mine = append(mine, line)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("want two completion lines for /api/v1/auth/me, got %d:\n%s", len(mine), h.logs.String())
	}

	first, second := mine[0], mine[1]
	for _, line := range mine {
		if id, _ := line["request_id"].(string); id == "" {
			t.Fatalf("the completion line carries no request_id: %v", line)
		}
	}
	if first["tenant_id"] != tenantA || first["user_id"] != userA {
		t.Fatalf("first line = %v, want the identity of practice A (%s, %s)", first, tenantA, userA)
	}
	if second["tenant_id"] != tenantB || second["user_id"] != userB {
		t.Fatalf("second line = %v, want the identity of practice B (%s, %s): a next request must start from an empty holder", second, tenantB, userB)
	}
}

// mustIssueToken mints a token for an explicit user id, so the tests know the
// values they must later see in the log.
func mustIssueToken(t *testing.T, h *harness, userID, tenantID string, role iam.RoleCode) string {
	t.Helper()

	token, err := h.tokens.Issue(authn.AccessClaims{Subject: userID, TenantID: tenantID, Role: string(role)})
	if err != nil {
		t.Fatalf("cannot issue the token: %v", err)
	}
	return token
}

package http_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	httpiface "github.com/niagga/aveniq/backend/internal/interfaces/http"
	"github.com/niagga/aveniq/backend/internal/platform/metrics"
)

// The measurement middleware is wired into the real router: every request the
// public router answers — feasible or refused — must be counted by route pattern,
// and /metrics must never be answerable from it. The registry itself is windowed
// by a second listener the composition root owns; this file pins only the public
// router's side of the contract.

func metricsRouter(t *testing.T, reg *metrics.Registry) http.Handler {
	t.Helper()

	now := time.Now()
	tenants := &fakeTenantRepository{byID: map[tenant.ID]tenant.Tenant{}}
	practice, err := tenant.New(tenant.ID(uuid.NewString()), "practica-metricas", "practica-metricas", now)
	if err != nil {
		t.Fatalf("cannot build the tenant: %v", err)
	}
	tenants.byID[practice.ID] = practice

	return httpiface.NewRouter(httpiface.Deps{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		ServiceVersion: "test",
		TrialExpired:   tenantapp.NewTrialExpiredUseCase(tenants, fixedClock{now: now}),
		Metrics:        reg,
	})
}

func scrapeRegistry(t *testing.T, reg *metrics.Registry) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	reg.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return recorder.Body.String()
}

func expectLine(t *testing.T, body, want string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if line == want {
			return
		}
	}
	t.Fatalf("exposition is missing the sample %q:\n%s", want, body)
}

func TestMetricsRecordOnThePublicRouter(t *testing.T) {
	reg := metrics.New()
	router := metricsRouter(t, reg)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("/healthz = %d, want 200", recorder.Code)
	}

	// A patient path with a raw identifier stays one series: the label is the
	// route pattern, not the URL the caller invented.
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/patients/"+uuid.NewString(), nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("patient without a token = %d, want 401", recorder.Code)
	}

	body := scrapeRegistry(t, reg)
	expectLine(t, body, `http_requests_total{method="GET",route="/healthz",status="200"} 1`)
	expectLine(t, body, `http_requests_total{method="GET",route="/api/v1/patients/{id}",status="401"} 1`)
	expectLine(t, body, `http_requests_inflight 0`)
	if _, found := samplePiece(body, `http_request_duration_seconds_sum{route="/healthz"}`); !found {
		t.Fatalf("no duration recorded for /healthz:\n%s", body)
	}
}

func TestMetricsIsNotOnThePublicRouter(t *testing.T) {
	reg := metrics.New()
	router := metricsRouter(t, reg)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("/metrics on the public router = %d, want 404: a tenant must not read the practice's internals", recorder.Code)
	}

	body := scrapeRegistry(t, reg)
	if strings.Contains(body, `route="/metrics"`) {
		t.Fatalf("the refused /metrics attempt became its own route series:\n%s", body)
	}
}

// samplePiece is a prefix-only variant for samples whose value held before the line ends.
func samplePiece(body, prefix string) (string, bool) {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line, true
		}
	}
	return "", false
}

package metrics_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/niagga/aveniq/backend/internal/platform/metrics"
)

// These tests pin the measurement contract: counters move when a request passes
// through, Prometheus can parse the exposition, and the shape never grows with
// the number of raw URLs a tenant can invent.

// expositionLine returns the exposition the registry serves, parsed into its
// non-empty lines.
func expositionLines(t *testing.T, reg *metrics.Registry) []string {
	t.Helper()
	recorder := httptest.NewRecorder()
	reg.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("scrape status = %d, want 200", recorder.Code)
	}
	var lines []string
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func sampleFor(lines []string, prefix string) (string, bool) {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return line, true
		}
	}
	return "", false
}

func requireSample(t *testing.T, lines []string, want string) {
	t.Helper()
	if _, found := sampleFor(lines, want); !found {
		t.Fatalf("exposition is missing %q; got:\n%s", want, strings.Join(lines, "\n"))
	}
}

func forbidSample(t *testing.T, lines []string, prefix string) {
	t.Helper()
	if _, found := sampleFor(lines, prefix); found {
		t.Fatalf("exposition contains a %q line; got:\n%s", prefix, strings.Join(lines, "\n"))
	}
}

func TestRequestsTotalMovesWithEachRequest(t *testing.T) {
	reg := metrics.New()
	chain := reg.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for range 2 {
		chain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/anything", nil))
	}
	chain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/anything", nil))

	lines := expositionLines(t, reg)
	requireSample(t, lines, `http_requests_total{method="GET",route="unmatched",status="200"} 2`)
	requireSample(t, lines, `http_requests_total{method="POST",route="unmatched",status="200"} 1`)
}

func TestRoutePatternNeverContainsTheRawPath(t *testing.T) {
	reg := metrics.New()
	router := chi.NewRouter()
	router.Use(reg.Middleware())
	router.Get("/api/v1/patients/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, id := range []string{"111", "222", "333"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/patients/"+id, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %s = %d, want 200", id, recorder.Code)
		}
	}

	lines := expositionLines(t, reg)
	requireSample(t, lines, `http_requests_total{method="GET",route="/api/v1/patients/{id}",status="200"} 3`)
	forbidSample(t, lines, `http_requests_total{method="GET",route="/api/v1/patients/111"`)
}

func TestDurationSumAndCountPerRoute(t *testing.T) {
	reg := metrics.New()
	router := chi.NewRouter()
	router.Use(reg.Middleware())
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	lines := expositionLines(t, reg)
	requireSample(t, lines, `http_request_duration_seconds_count{route="/healthz"} 1`)
	sumLine, found := sampleFor(lines, `http_request_duration_seconds_sum{route="/healthz"} `)
	if !found {
		t.Fatalf("missing duration sum; got:\n%s", strings.Join(lines, "\n"))
	}
	fields := strings.Fields(sumLine)
	if len(fields) != 2 {
		t.Fatalf("sum line %q is not `name value`", sumLine)
	}
	seconds, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || seconds <= 0 {
		t.Fatalf("sum value %q is not a positive number: %v", fields[1], err)
	}
}

func TestInflightGaugeTracksServingRequests(t *testing.T) {
	reg := metrics.New()
	block := make(chan struct{})
	chain := reg.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { <-block }))
	serving := make(chan struct{})
	go func() {
		close(serving)
		chain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	}()
	<-serving
	defer close(block)

	deadline := time.Now().Add(time.Second)
	for reg.Inflight() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("inflight = %d while a request is being served, want 1", reg.Inflight())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestExpositionShapeIsWhatPrometheusParses(t *testing.T) {
	reg := metrics.New()
	router := chi.NewRouter()
	router.Use(reg.Middleware())
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	recorder := httptest.NewRecorder()
	reg.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if ct := recorder.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}

	body := recorder.Body.String()
	for _, want := range []string{
		"# HELP http_requests_total ",
		"# TYPE http_requests_total counter",
		"# HELP http_request_duration_seconds ",
		"# TYPE http_request_duration_seconds summary",
		`http_request_duration_seconds_sum{route="/healthz"} `,
		`http_request_duration_seconds_count{route="/healthz"} `,
		"# HELP http_requests_inflight ",
		"# TYPE http_requests_inflight gauge",
		"http_requests_inflight 0",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("exposition is missing %q:\n%s", want, body)
		}
	}
}

// TestScrapeServerServesMetricsOnly pins the second listener: it is a private
// window, not a second API.
func TestScrapeServerServesMetricsOnly(t *testing.T) {
	reg := metrics.New()
	server := metrics.Server("127.0.0.1:0", reg)
	listener, err := serveOnAddress(server)
	if err != nil {
		t.Fatalf("cannot start the metrics listener: %v", err)
	}
	defer server.Close()
	base := "http://" + listener.Addr().String()

	res, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("/metrics = %d, want 200", res.StatusCode)
	}
	if body := string(body); !strings.Contains(body, "# TYPE http_requests_inflight gauge") {
		t.Fatalf("/metrics did not render the registry:\n%s", body)
	}

	res, err = http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz on the metrics listener: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("/healthz on the metrics listener = %d, want 404", res.StatusCode)
	}
}

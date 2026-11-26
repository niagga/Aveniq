// Package metrics holds the process' own measurement: HTTP request counters,
// per-route latency totals, an in-flight gauge, and gauges contributed at scrape
// time by late collectors such as the queue inspector. The exposition is the
// text format Prometheus scrapes (version 0.0.4), rendered by hand so this
// stays a no-new-dependency platform package.
package metrics

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// The exposition speaks one dialect: Prometheus text format 0.0.4. The charset
// is declared so a scraper forwarding the body to a human can still read it.
const expositionContentType = "text/plain; version=0.0.4; charset=utf-8"

// unmatchedRoute labels the series of a request chi could not place under a
// route (the 404 and 405 fallthroughs). Without it those requests would either
// be dropped from the counts or share a raw-path label the tenants can invent.
const unmatchedRoute = "unmatched"

// requestKey is the identity of one requests_total series. Keeping it a struct
// of strings means one map lookup per request and no label concatenation on the
// hot path.
type requestKey struct {
	method string
	route  string
	status int
}

type counter struct{ total atomic.Uint64 }

type durations struct {
	// nanos is the sum as an integer an atomic can add: floats are not
	// atomically addable without a lock, and nanosecond precision is far above
	// what the exposition or a rate alert needs.
	nanos atomic.Int64
	count atomic.Uint64
}

// Registry is the volatile measurement state of one process. Recording costs
// atomic increments plus at most one mutex-guarded map insertion per request;
// a scrape reads atomics and never holds the lock while rendering.
type Registry struct {
	mu        sync.Mutex
	requests  map[requestKey]*counter
	durations map[string]*durations
	inflight  atomic.Int64
	scrapeErr atomic.Uint64
	queue     QueueSource
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{
		requests:  make(map[requestKey]*counter),
		durations: make(map[string]*durations),
	}
}

// RecordQueueSource registers the late collector read at every scrape. Queue
// depths are too volatile to sample inside the request path and too cheap to
// read once to keep them warm in the background: scrape time is when the
// operator is looking at them.
func (reg *Registry) RecordQueueSource(source QueueSource) {
	reg.mu.Lock()
	reg.queue = source
	reg.mu.Unlock()
}

// Inflight reports how many requests the middleware is currently serving.
func (reg *Registry) Inflight() int64 { return reg.inflight.Load() }

// Middleware records every answer the wrapped chain produces, keyed by the
// route pattern chi matched — never the raw URL, or every patient id becomes a
// new time series. Install it after RequestID so the request log lines and the
// counters agree on which request is which.
func (reg *Registry) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reg.inflight.Add(1)
			defer reg.inflight.Add(-1)

			started := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(wrapped, r)

			route := unmatchedRoute
			// The pattern exists only once chi matched the request; reading it
			// after next.ServeHTTP returns is why recording happens on exit.
			if rc := chi.RouteContext(r.Context()); rc != nil {
				if pattern := rc.RoutePattern(); pattern != "" {
					route = pattern
				}
			}
			reg.record(r.Method, route, wrapped.Status(), time.Since(started))
		})
	}
}

func (reg *Registry) record(method, route string, status int, elapsed time.Duration) {
	key := requestKey{method: method, route: route, status: status}

	reg.mu.Lock()
	counterEntry, ok := reg.requests[key]
	if !ok {
		counterEntry = &counter{}
		reg.requests[key] = counterEntry
	}
	durationEntry, ok := reg.durations[route]
	if !ok {
		durationEntry = &durations{}
		reg.durations[route] = durationEntry
	}
	reg.mu.Unlock()

	counterEntry.total.Add(1)
	durationEntry.count.Add(1)
	durationEntry.nanos.Add(int64(elapsed))
}

// requestSample is one rendered-on-demand requests_total row.
type requestSample struct {
	key   requestKey
	count uint64
}

// durationSample is one rendered-on-demand per-route latency row.
type durationSample struct {
	route string
	nanos int64
	count uint64
}

// scrapeSnapshot is the consistent-enough view of one scrape: each value is
// read in one atomic load, so a scrape is internally timestamp-ordered by the
// order these loads happen and exact per value.
type scrapeSnapshot struct {
	requests         []requestSample
	durationsByRoute []durationSample
	inflight         int64
	queueSamples     []QueueSample
	queueErrored     bool
}

// snapshot copies the registry state and reads the late queue collector. On a
// queue read failure the samples are dropped and the scrape error counter is
// incremented: an omitted gauge cannot be mistaken for an empty queue.
func (reg *Registry) snapshot() scrapeSnapshot {
	snap := scrapeSnapshot{inflight: reg.inflight.Load()}

	reg.mu.Lock()
	requests := make([]requestSample, 0, len(reg.requests))
	for key, entry := range reg.requests {
		requests = append(requests, requestSample{key: key, count: entry.total.Load()})
	}
	durationsByRoute := make([]durationSample, 0, len(reg.durations))
	for route, entry := range reg.durations {
		durationsByRoute = append(durationsByRoute, durationSample{
			route: route, nanos: entry.nanos.Load(), count: entry.count.Load(),
		})
	}
	reg.mu.Unlock()

	sortSamples(requests)
	sortDurationSamples(durationsByRoute)

	snap.requests = requests
	snap.durationsByRoute = durationsByRoute

	reg.mu.Lock()
	source := reg.queue
	reg.mu.Unlock()
	if source != nil {
		samples, err := source.Sample()
		if err != nil {
			reg.scrapeErr.Add(1)
			snap.queueErrored = true
		} else {
			snap.queueSamples = samples
		}
	}

	return snap
}

// sortSamples orders the requests_total rows by route, method, then status, so
// two consecutive scrapes of the same state are byte-identical: diffing an
// exposition must never answer "everything changed".
func sortSamples(samples []requestSample) {
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].key.route != samples[j].key.route {
			return samples[i].key.route < samples[j].key.route
		}
		if samples[i].key.method != samples[j].key.method {
			return samples[i].key.method < samples[j].key.method
		}
		return samples[i].key.status < samples[j].key.status
	})
}

func sortDurationSamples(samples []durationSample) {
	sort.Slice(samples, func(i, j int) bool { return samples[i].route < samples[j].route })
}

// Handler serves the exposition. It is the whole contract of the private
// listener: one route, one content type, one snapshot per scrape.
func (reg *Registry) Handler() http.Handler {
	return http.HandlerFunc(reg.writeExposition)
}

func (reg *Registry) writeExposition(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", expositionContentType)
	snap := reg.snapshot()

	var out strings.Builder
	writeHelp(&out, "http_requests_total", "counter",
		"HTTP requests answered, partitioned by method, route pattern and response status.")
	for _, sample := range snap.requests {
		writeSeries(&out, "http_requests_total",
			"method="+escapeLabel(sample.key.method)+",route="+escapeLabel(sample.key.route)+
				",status="+escapeLabel(strconv.Itoa(sample.key.status)),
			strconv.FormatUint(sample.count, 10))
	}

	writeHelp(&out, "http_request_duration_seconds", "summary",
		"Cumulative seconds spent answering HTTP requests, summed by route pattern.")
	for _, sample := range snap.durationsByRoute {
		writeSeries(&out, "http_request_duration_seconds_sum",
			"route="+escapeLabel(sample.route),
			strconv.FormatFloat(float64(sample.nanos)/float64(time.Second), 'f', -1, 64))
		writeSeries(&out, "http_request_duration_seconds_count",
			"route="+escapeLabel(sample.route),
			strconv.FormatUint(sample.count, 10))
	}

	writeHelp(&out, "http_requests_inflight", "gauge",
		"HTTP requests currently being served.")
	writeSeries(&out, "http_requests_inflight", "", strconv.FormatInt(snap.inflight, 10))

	if snap.queueErrored || snap.queueSamples != nil {
		writeQueueFamilies(&out, snap.queueSamples)
		writeHelp(&out, "asynq_queue_scrape_errors_total", "counter",
			"Scrapes whose queue read failed or timed out; those scrapes omit the queue samples.")
		writeSeries(&out, "asynq_queue_scrape_errors_total", "",
			strconv.FormatUint(reg.scrapeErr.Load(), 10))
	}

	_, _ = w.Write([]byte(out.String()))
}

func writeQueueFamilies(out *strings.Builder, samples []QueueSample) {
	writeHelp(out, "asynq_queue_pending", "gauge",
		"Tasks waiting to be picked up, per queue.")
	writeHelp(out, "asynq_queue_active", "gauge",
		"Tasks currently being processed, per queue.")
	writeHelp(out, "asynq_queue_retry", "gauge",
		"Tasks that failed and are waiting to be retried, per queue.")
	writeHelp(out, "asynq_queue_failed", "gauge",
		"Task processing failures asynq counted for the day, per queue (it resets daily).")
	for _, sample := range samples {
		labels := "queue=" + escapeLabel(sample.Queue)
		writeSeries(out, "asynq_queue_pending", labels, strconv.Itoa(sample.Pending))
		writeSeries(out, "asynq_queue_active", labels, strconv.Itoa(sample.Active))
		writeSeries(out, "asynq_queue_retry", labels, strconv.Itoa(sample.Retry))
		writeSeries(out, "asynq_queue_failed", labels, strconv.Itoa(sample.Failed))
	}
}

func writeSeries(out *strings.Builder, name, labels, value string) {
	out.WriteString(name)
	if labels != "" {
		out.WriteString("{" + labels + "}")
	}
	out.WriteString(" " + value + "\n")
}

func writeHelp(out *strings.Builder, name, kind, help string) {
	out.WriteString("# HELP " + name + " " + help + "\n# TYPE " + name + " " + kind + "\n")
}

// escapeLabel quotes a label value the exposition requires. HELP text is
// written literally by the package; only sample values pass through here.
func escapeLabel(value string) string {
	quoted := strings.ReplaceAll(value, `\`, `\\`)
	quoted = strings.ReplaceAll(quoted, `"`, `\"`)
	quoted = strings.ReplaceAll(quoted, "\n", `\n`)
	return `"` + quoted + `"`
}

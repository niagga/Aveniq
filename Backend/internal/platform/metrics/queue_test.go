package metrics_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/niagga/aveniq/backend/internal/platform/metrics"
)

// The queue depth must be read at scrape time — never cached into the registry —
// and a slow Redis must not hang the scrape: the read is bounded and its failure
// is exposed as an error counter while the samples are left out (an omitted
// number cannot be mistaken for an empty queue).

type fakeQueueSource struct {
	samples []metrics.QueueSample
	err     error
	delay   time.Duration
}

func (f *fakeQueueSource) Sample() ([]metrics.QueueSample, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.samples, f.err
}

func TestQueueGaugesRenderAtScrapeTime(t *testing.T) {
	reg := metrics.New()
	reg.RecordQueueSource(&fakeQueueSource{samples: []metrics.QueueSample{
		{Queue: "critical", Pending: 1, Active: 2, Retry: 3, Failed: 4},
		{Queue: "default", Pending: 10, Active: 0, Retry: 0, Failed: 0},
	}})

	lines := expositionLines(t, reg)
	requireSample(t, lines, `asynq_queue_pending{queue="default"} 10`)
	requireSample(t, lines, `asynq_queue_active{queue="default"} 0`)
	requireSample(t, lines, `asynq_queue_retry{queue="default"} 0`)
	requireSample(t, lines, `asynq_queue_failed{queue="default"} 0`)
	requireSample(t, lines, `asynq_queue_pending{queue="critical"} 1`)
	requireSample(t, lines, `asynq_queue_scrape_errors_total 0`)
}

func TestQueueReadFailureOmitsSamplesAndCountsTheError(t *testing.T) {
	reg := metrics.New()
	reg.RecordQueueSource(&fakeQueueSource{samples: []metrics.QueueSample{}, err: errors.New("redis down")})

	lines := expositionLines(t, reg)
	forbidSample(t, lines, "asynq_queue_pending")
	forbidSample(t, lines, "asynq_queue_active")
	forbidSample(t, lines, "asynq_queue_retry")
	forbidSample(t, lines, "asynq_queue_failed")
	requireSample(t, lines, "asynq_queue_scrape_errors_total 1")
}

func TestAsynqInspectorMapsQueueInfoOntoSamples(t *testing.T) {
	inspector := fakeInspector{queues: []string{"critical", "default"}, info: map[string]*asynq.QueueInfo{
		"default":  {Queue: "default", Pending: 3, Active: 1, Retry: 2, Failed: 5},
		"critical": {Queue: "critical"},
	}}
	source := metrics.NewAsynqQueueSource(inspector, []string{"critical", "default"}, time.Second)

	samples, err := source.Sample()
	if err != nil {
		t.Fatalf("sample failed: %v", err)
	}
	want := []metrics.QueueSample{
		{Queue: "critical"},
		{Queue: "default", Pending: 3, Active: 1, Retry: 2, Failed: 5},
	}
	if len(samples) != len(want) {
		t.Fatalf("samples = %+v, want %+v", samples, want)
	}
	for i := range want {
		if samples[i] != want[i] {
			t.Fatalf("sample %d = %+v, want %+v", i, samples[i], want[i])
		}
	}
}

// TestAsynqInspectorReadsANeverCreatedQueueAsEmpty pins asynq's lazy queue
// reification: a name this worker owns but that never received a task is
// absent from the Redis queue list. It is genuinely empty — zeros — not a
// scrape failure, or an idle deployment would show nothing but scrape errors.
func TestAsynqInspectorReadsANeverCreatedQueueAsEmpty(t *testing.T) {
	inspector := fakeInspector{} // Redis holds no queues at all
	source := metrics.NewAsynqQueueSource(inspector, []string{"critical", "default"}, time.Second)

	samples, err := source.Sample()
	if err != nil {
		t.Fatalf("sample failed on a fresh Redis: %v", err)
	}
	want := []metrics.QueueSample{{Queue: "critical"}, {Queue: "default"}}
	if len(samples) != len(want) {
		t.Fatalf("samples = %+v, want one empty sample per queue", samples)
	}
	for i := range want {
		if samples[i] != want[i] {
			t.Fatalf("sample %d = %+v, want %+v", i, samples[i], want[i])
		}
	}
}

func TestAsynqInspectorIsBoundedByATimeout(t *testing.T) {
	hang := fakeInspector{
		queues: []string{"default"},
		info:   map[string]*asynq.QueueInfo{"default": {Queue: "default", Pending: 7}},
		delay:  200 * time.Millisecond,
	}
	source := metrics.NewAsynqQueueSource(hang, []string{"default"}, 10*time.Millisecond)

	timedOut := make(chan error, 1)
	go func() {
		_, err := source.Sample()
		timedOut <- err
	}()
	select {
	case err := <-timedOut:
		if err == nil {
			t.Fatal("a hanging inspector read returned no error; the scrape would hang with it")
		}
	case <-time.After(time.Second):
		t.Fatal("Sample() blocked past its timeout")
	}
}

func TestAsynqInspectorFailsWhenReadErrors(t *testing.T) {
	inspector := fakeInspector{err: errors.New("connection refused")}
	source := metrics.NewAsynqQueueSource(inspector, []string{"default"}, time.Second)

	if samples, err := source.Sample(); err == nil {
		t.Fatalf("Sample() = %+v without error, want the inspector error", samples)
	}
}

// fakeInspector stands in for asynq.Inspector: the adapter takes the method set
// it actually needs, so this fake needs no Redis.
type fakeInspector struct {
	queues []string
	info   map[string]*asynq.QueueInfo
	err    error
	delay  time.Duration
}

func (f fakeInspector) Queues() ([]string, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return nil, fmt.Errorf("couldn't list queues: %w", f.err)
	}
	return f.queues, nil
}

func (f fakeInspector) GetQueueInfo(queue string) (*asynq.QueueInfo, error) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return nil, fmt.Errorf("couldn't read queue %q: %w", queue, f.err)
	}
	if info, ok := f.info[queue]; ok {
		return info, nil
	}
	return nil, fmt.Errorf("asynq: %w", asynq.ErrQueueNotFound)
}

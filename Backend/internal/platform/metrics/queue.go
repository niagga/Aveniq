package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/hibiken/asynq"
)

// QueueSample is one queue's depth, read at scrape time and rendered as gauges.
type QueueSample struct {
	Queue   string
	Pending int
	Active  int
	Retry   int
	Failed  int
}

// QueueSource reads the volatile depths this process does not own: the registry
// never caches them, so every scrape answers with the moment it rendered.
type QueueSource interface {
	Sample() ([]QueueSample, error)
}

// QueueInfoReader is the slice of asynq.Inspector the adapter needs. Declaring
// the interface here keeps the queue tests Redis-free and lets the concrete
// Inspector be passed by the composition root unchanged.
type QueueInfoReader interface {
	// Queues lists the queues asynq currently holds in Redis.
	Queues() ([]string, error)
	// GetQueueInfo reads one queue's stats.
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
}

// asynqQueueSource adapts an asynq Inspector onto the QueueSource contract.
type asynqQueueSource struct {
	inspector QueueInfoReader
	queues    []string
	timeout   time.Duration
}

// NewAsynqQueueSource reads the given queues through the Inspector at scrape
// time. The timeout bounds the whole read: a down or slow Redis must make the
// scrape fail visibly in seconds, not hang the scraper forever.
func NewAsynqQueueSource(inspector QueueInfoReader, queues []string, timeout time.Duration) QueueSource {
	return &asynqQueueSource{inspector: inspector, queues: queues, timeout: timeout}
}

// Sample reads every queue in parallel and waits at most the configured timeout
// for the whole batch. One failed or slow queue fails the scrape: a partially
// reported Redis state reads like the other half is empty.
//
// asynq reifies a queue on its first task, so a name this worker owns but that
// has never received one is simply absent from Redis's queue list. That is a
// genuinely empty queue — zeros — not a scrape failure, or an idle deployment
// would show nothing but scrape errors and nothing could ever alert on them.
// A connection or command failure IS a failure, and fails the scrape.
func (s *asynqQueueSource) Sample() ([]QueueSample, error) {
	type read struct {
		sample QueueSample
		err    error
	}

	existing, err := s.inspector.Queues()
	if err != nil {
		return nil, fmt.Errorf("listing queues: %w", err)
	}
	live := make(map[string]bool, len(existing))
	for _, queue := range existing {
		live[queue] = true
	}

	results := make(chan read, len(s.queues))
	for _, queue := range s.queues {
		name := queue
		if !live[name] {
			// Absent from Redis: zero depth for every state.
			results <- read{sample: QueueSample{Queue: name}}
			continue
		}
		go func() {
			info, err := s.inspector.GetQueueInfo(name)
			if err != nil {
				results <- read{err: fmt.Errorf("reading queue %q: %w", name, err)}
				return
			}
			results <- read{sample: QueueSample{
				Queue:   name,
				Pending: info.Pending,
				Active:  info.Active,
				Retry:   info.Retry,
				Failed:  info.Failed,
			}}
		}()
	}

	samples := make([]QueueSample, 0, len(s.queues))
	deadline := time.After(s.timeout)
	for range len(s.queues) {
		select {
		case result := <-results:
			if result.err != nil {
				return nil, result.err
			}
			samples = append(samples, result.sample)
		case <-deadline:
			return nil, fmt.Errorf("queue scrape exceeded its %s timeout", s.timeout)
		}
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i].Queue < samples[j].Queue })
	return samples, nil
}

// Server holds the scrape listener's handler configuration: /metrics and
// nothing else. The address config is off by default and the process must not
// serve its internals unless the operator asked; this server only ever answers
// one route on one port that has no tenant-facing duty.
func Server(addr string, reg *Registry) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", reg.Handler().ServeHTTP)

	// A generous write timeout: the queue read has its own bound, so a scrape
	// can only be held up by that, never by a client we wait for forever.
	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

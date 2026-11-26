// Command worker is the composition root of the background process.
//
// It runs the same binary family as the API but with a different responsibility:
// consuming the Redis-backed queue. Fiscal submissions to ARCA, invoice PDF
// rendering and transactional email will register their handlers here.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/niagga/aveniq/backend/internal/platform/config"
	"github.com/niagga/aveniq/backend/internal/platform/logging"
	"github.com/niagga/aveniq/backend/internal/platform/metrics"
)

const (
	// TaskMaintenanceNoop proves the queue wiring end to end. It exists so a
	// broken Redis connection or scheduler fails loudly during setup rather than
	// after the first real job is deployed.
	TaskMaintenanceNoop = "maintenance:noop"

	queueDefault  = "default"
	queueCritical = "critical"
	concurrency   = 4
	shutdownGrace = 20 * time.Second

	// queueScrapeTimeout bounds an Inspector read at scrape time. It must be
	// comfortably under a Prometheus scrape deadline: a down Redis makes the
	// scrape fail in seconds instead of hanging the scraper forever.
	queueScrapeTimeout = 2 * time.Second

	metricsShutdownGrace = 5 * time.Second
)

var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		slog.Error("worker: fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.Env, cfg.LogLevel, "worker")
	slog.SetDefault(logger)

	// The queue depths are gauges read at scrape time straight from the asynq
	// Inspector — never cached — so a scrape always answers with the moment it
	// rendered. A failed read bounds itself and shows up as an error counter
	// while the samples stay out: a missing number cannot be mistaken for an
	// empty queue.
	workerMetrics := metrics.New()
	workerMetrics.RecordQueueSource(metrics.NewAsynqQueueSource(
		asynq.NewInspector(asynq.RedisClientOpt{Addr: cfg.RedisAddr}),
		[]string{queueDefault, queueCritical},
		queueScrapeTimeout,
	))

	// METRICS_ADDR unset means no listener at all, the default everywhere: the
	// scrape socket is an operator's explicit opt-in, never an accident.
	serveErr := make(chan error, 1)
	var scrapeServer *http.Server
	if cfg.MetricsAddr != "" {
		scrapeServer = metrics.Server(cfg.MetricsAddr, workerMetrics)
		go func() {
			logger.Info("worker: metrics listening", slog.String("addr", cfg.MetricsAddr))
			if err := scrapeServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serveErr <- fmt.Errorf("worker: metrics server failed: %w", err)
			}
		}()
	}

	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: concurrency,
			// Queues are weighted: critical work drains before bulk work.
			Queues: map[string]int{
				queueCritical: 6,
				queueDefault:  10,
			},
			Logger: asynqLogger{logger: logger},
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskMaintenanceNoop, handleMaintenanceNoop(logger))

	if err := server.Start(mux); err != nil {
		return fmt.Errorf("worker: cannot start: %w", err)
	}

	logger.Info("worker: started",
		slog.String("redis", cfg.RedisAddr),
		slog.Int("concurrency", concurrency),
		slog.String("version", version),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("worker: shutdown requested")
	}

	server.Shutdown()
	if scrapeServer != nil {
		scrapeCtx, cancel := context.WithTimeout(context.Background(), metricsShutdownGrace)
		defer cancel()
		if err := scrapeServer.Shutdown(scrapeCtx); err != nil {
			return fmt.Errorf("worker: metrics server shutdown failed: %w", err)
		}
	}
	logger.Info("worker: stopped")
	return nil
}

func handleMaintenanceNoop(logger *slog.Logger) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		logger.InfoContext(ctx, "worker: task received",
			slog.String("type", task.Type()),
			slog.String("payload", string(task.Payload())),
		)
		return nil
	}
}

// asynqLogger adapts asynq's logger interface onto slog so background logs share
// one format and one destination with the rest of the process.
type asynqLogger struct{ logger *slog.Logger }

func (l asynqLogger) Debug(args ...any) { l.logger.Debug(fmt.Sprint(args...)) }
func (l asynqLogger) Info(args ...any)  { l.logger.Info(fmt.Sprint(args...)) }
func (l asynqLogger) Warn(args ...any)  { l.logger.Warn(fmt.Sprint(args...)) }
func (l asynqLogger) Error(args ...any) { l.logger.Error(fmt.Sprint(args...)) }
func (l asynqLogger) Fatal(args ...any) { l.logger.Error(fmt.Sprint(args...)) }

func (l asynqLogger) Debugf(format string, args ...any) { l.logger.Debug(fmt.Sprintf(format, args...)) }
func (l asynqLogger) Infof(format string, args ...any)  { l.logger.Info(fmt.Sprintf(format, args...)) }
func (l asynqLogger) Warnf(format string, args ...any)  { l.logger.Warn(fmt.Sprintf(format, args...)) }
func (l asynqLogger) Errorf(format string, args ...any) { l.logger.Error(fmt.Sprintf(format, args...)) }
func (l asynqLogger) Fatalf(format string, args ...any) { l.logger.Error(fmt.Sprintf(format, args...)) }

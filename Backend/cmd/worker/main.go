// Command worker is the composition root of the background process.
//
// It runs the same binary family as the API but with a different responsibility:
// consuming the Redis-backed queue. Fiscal submissions to ARCA, invoice PDF
// rendering and transactional email will register their handlers here.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/niagga/aveniq/backend/internal/platform/config"
	"github.com/niagga/aveniq/backend/internal/platform/logging"
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
	<-ctx.Done()

	logger.Info("worker: shutdown requested")
	server.Shutdown()
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

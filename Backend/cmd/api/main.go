// Command api is the composition root of the HTTP process.
//
// Wiring lives here and nowhere else: infrastructure adapters are built, injected
// into use cases, and those use cases are handed to the delivery layer. No other
// file decides which implementation powers which port.
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

	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
	"github.com/niagga/aveniq/backend/internal/infrastructure/redisx"
	httpiface "github.com/niagga/aveniq/backend/internal/interfaces/http"
	"github.com/niagga/aveniq/backend/internal/platform/clock"
	"github.com/niagga/aveniq/backend/internal/platform/config"
	"github.com/niagga/aveniq/backend/internal/platform/idgen"
	"github.com/niagga/aveniq/backend/internal/platform/logging"
)

// version is overridden at build time with:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
var version = "0.1.0-dev"

const (
	shutdownTimeout   = 20 * time.Second
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("api: fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.Env, cfg.LogLevel, "api")
	slog.SetDefault(logger)

	// A cancellation context tied to the signals a container runtime or a
	// developer sends is the only correct way to stop a server gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	redisClient, err := redisx.NewClient(ctx, cfg.RedisAddr)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()

	// ─── Adapters → ports ───
	tenantRepository := postgres.NewTenantRepository(pool)

	// ─── Ports → use cases ───
	deps := httpiface.Deps{
		Logger:         logger,
		ServiceVersion: version,
		CreateTenant:   tenantapp.NewCreateUseCase(tenantRepository, clock.New(), idgen.UUIDGenerator{}),
		ListTenants:    tenantapp.NewListUseCase(tenantRepository),
		GetTenant:      tenantapp.NewGetBySlugUseCase(tenantRepository),
		ReadyChecks: []httpiface.ReadyCheck{
			{Name: "postgres", Check: pool.Ping},
			{Name: "redis", Check: func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }},
		},
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpiface.NewRouter(deps),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("api: listening",
			slog.String("addr", cfg.HTTPAddr),
			slog.String("env", string(cfg.Env)),
			slog.String("version", version),
		)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("api: server failed: %w", err)
	case <-ctx.Done():
		logger.Info("api: shutdown requested")
	}

	// A fresh context: the signal context is already cancelled, and the shutdown
	// itself still needs time to drain in-flight requests.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("api: graceful shutdown failed: %w", err)
	}

	logger.Info("api: stopped")
	return nil
}

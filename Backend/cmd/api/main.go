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

	authapp "github.com/niagga/aveniq/backend/internal/application/auth"
	patientapp "github.com/niagga/aveniq/backend/internal/application/patient"
	tenantapp "github.com/niagga/aveniq/backend/internal/application/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
	"github.com/niagga/aveniq/backend/internal/infrastructure/redisx"
	httpiface "github.com/niagga/aveniq/backend/internal/interfaces/http"
	"github.com/niagga/aveniq/backend/internal/platform/authn"
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
	// accessTokenTTL is deliberately short: E1 has no access-token revocation, so
	// the only bound on a stolen token is its expiry (decision 5 of L11).
	accessTokenTTL = 15 * time.Minute
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
	userRepository := postgres.NewUserRepository(pool)
	sessionRepository := postgres.NewSessionRepository(pool)
	patientRepository := postgres.NewPatientRepository(pool)
	patientCatalog := postgres.NewPatientCatalog(pool)
	transactor := postgres.NewTransactor(pool)

	// ─── Authentication primitives ───
	hasher, err := authn.NewPasswordHasher(authn.DefaultArgon2idParams())
	if err != nil {
		return err
	}

	// The dummy hash is minted once, here, so the login path can spend the same
	// verification time for an address that does not exist.
	dummyHash, err := hasher.Hash(idgen.UUIDGenerator{}.NewID())
	if err != nil {
		return err
	}

	accessTokens, err := authn.NewAccessTokens([]byte(cfg.AccessTokenSecret), accessTokenTTL, time.Now)
	if err != nil {
		return err
	}

	loginUseCase, err := authapp.NewLoginUseCase(authapp.LoginDeps{
		Tenants:   tenantRepository,
		Users:     userRepository,
		Sessions:  sessionRepository,
		Scope:     transactor,
		Verifier:  hasher,
		Tokens:    accessTokens,
		Clock:     clock.New(),
		IDs:       idgen.UUIDGenerator{},
		DummyHash: dummyHash,
	})
	if err != nil {
		return err
	}

	// ─── Ports → use cases ───
	deps := httpiface.Deps{
		Logger:            logger,
		ServiceVersion:    version,
		CreateTenant:      tenantapp.NewCreateUseCase(tenantRepository, clock.New(), idgen.UUIDGenerator{}),
		GetTenantByID:     tenantapp.NewGetByIDUseCase(tenantRepository),
		Login:             loginUseCase,
		CreatePatient:     patientapp.NewCreateUseCase(patientRepository, patientCatalog, transactor, clock.New(), idgen.UUIDGenerator{}),
		UpdatePatient:     patientapp.NewUpdateUseCase(patientRepository, patientCatalog, transactor, clock.New()),
		DeactivatePatient: patientapp.NewDeactivateUseCase(patientRepository, transactor, clock.New()),
		ActivatePatient:   patientapp.NewActivateUseCase(patientRepository, transactor, clock.New()),
		GetPatient:        patientapp.NewGetUseCase(patientRepository, transactor),
		ListPatients:      patientapp.NewListUseCase(patientRepository, transactor),
		Refresh: authapp.NewRefreshUseCase(authapp.RefreshDeps{
			Users:    userRepository,
			Sessions: sessionRepository,
			Scope:    transactor,
			Tokens:   accessTokens,
			Clock:    clock.New(),
			IDs:      idgen.UUIDGenerator{},
		}),
		Logout: authapp.NewLogoutUseCase(authapp.LogoutDeps{
			Sessions: sessionRepository,
			Scope:    transactor,
			Clock:    clock.New(),
		}),
		AccessTokens: accessTokens,
		// The refresh grant is a browser cookie: HttpOnly so JavaScript cannot read
		// it, and Secure everywhere except the local plain-HTTP loopback.
		RefreshCookie: httpiface.DefaultRefreshCookieConfig(authapp.RefreshSessionTTL, !cfg.IsDevelopment()),
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

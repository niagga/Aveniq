// Package postgres hosts the PostgreSQL adapters: the only place in the codebase
// that knows SQL, pgx or the generated query layer.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool tuning. Values are intentionally modest: a single API instance serving a
// handful of tenants does not need a large pool, and a small one keeps the
// database's connection budget free for migrations and background jobs.
const (
	maxConnections    = 10
	minConnections    = 1
	maxConnLifetime   = time.Hour
	maxConnIdleTime   = 30 * time.Minute
	healthCheckPeriod = time.Minute
	pingTimeout       = 10 * time.Second
)

// NewPool builds a connection pool and verifies it can reach the database.
// Failing here is better than starting a process that cannot serve a request.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: invalid DATABASE_URL: %w", err)
	}

	cfg.MaxConns = maxConnections
	cfg.MinConns = minConnections
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.HealthCheckPeriod = healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: cannot create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: cannot reach the database: %w", err)
	}

	// Row-level security is the only thing separating one tenant's data from
	// another's, and PostgreSQL lets a SUPERUSER or a role holding BYPASSRLS
	// ignore every policy. That is not hypothetical: the local cluster's
	// original role had both attributes, which made the entire IAM isolation
	// layer decorative while every test still passed. Refusing to start turns
	// that silent catastrophe into a startup failure.
	var restricted bool
	if err := pool.QueryRow(pingCtx, `
		SELECT NOT (rolsuper OR rolbypassrls)
		FROM pg_roles
		WHERE rolname = current_user`).Scan(&restricted); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: cannot inspect the connection role: %w", err)
	}
	if !restricted {
		pool.Close()
		return nil, errors.New("postgres: refusing to start: the connection role is a superuser or holds BYPASSRLS, which ignores row-level security and disables tenant isolation; connect as the application role instead")
	}

	return pool, nil
}

// Package config loads and validates runtime configuration from the environment.
// It fails fast: a process with unusable configuration must not start.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// AppEnv distinguishes the relaxed local setup from anything else.
type AppEnv string

const (
	EnvDevelopment AppEnv = "development"
	EnvProduction  AppEnv = "production"
	EnvTest        AppEnv = "test"
)

// Config is the fully resolved configuration of a process.
type Config struct {
	Env         AppEnv
	HTTPAddr    string
	DatabaseURL string
	RedisAddr   string
	LogLevel    slog.Level
	// BaseDomain is the registrable suffix used to resolve a tenant from the
	// request host (for example "aveniq.app" resolves sur.aveniq.app).
	BaseDomain string
}

// Load reads the environment, applying development-friendly defaults so a fresh
// clone runs against the hermetic local stack without any .env file.
func Load() (Config, error) {
	cfg := Config{
		Env:         AppEnv(strings.TrimSpace(getEnv("APP_ENV", string(EnvDevelopment)))),
		HTTPAddr:    getEnv("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://aveniq:aveniq_dev@127.0.0.1:5433/aveniq?sslmode=disable"),
		RedisAddr:   getEnv("REDIS_ADDR", "127.0.0.1:6380"),
		BaseDomain:  getEnv("BASE_DOMAIN", "localhost"),
	}

	level, err := parseLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}
	cfg.LogLevel = level

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	switch c.Env {
	case EnvDevelopment, EnvProduction, EnvTest:
	default:
		return fmt.Errorf("config: APP_ENV %q is not one of development, production, test", c.Env)
	}

	if c.DatabaseURL == "" {
		return fmt.Errorf("config: DATABASE_URL must not be empty")
	}
	if c.HTTPAddr == "" {
		return fmt.Errorf("config: HTTP_ADDR must not be empty")
	}
	if c.RedisAddr == "" {
		return fmt.Errorf("config: REDIS_ADDR must not be empty")
	}

	// TLS to the database is not optional outside development: silently talking
	// plaintext to Postgres in production is the kind of mistake that is only
	// discovered after an incident.
	if c.Env == EnvProduction && !strings.Contains(c.DatabaseURL, "sslmode=") {
		return fmt.Errorf("config: DATABASE_URL must set sslmode outside development")
	}

	return nil
}

// IsDevelopment reports whether relaxed behaviour is allowed.
func (c Config) IsDevelopment() bool { return c.Env == EnvDevelopment }

func parseLevel(raw string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(raw))); err != nil {
		return 0, fmt.Errorf("config: LOG_LEVEL %q is invalid: %w", raw, err)
	}
	return level, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

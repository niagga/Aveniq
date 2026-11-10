// Package logging builds the structured logger used by every process.
package logging

import (
	"log/slog"
	"os"

	"github.com/niagga/aveniq/backend/internal/platform/config"
)

// New returns a logger suited to the environment: human-readable text locally,
// machine-parseable JSON everywhere else.
func New(env config.AppEnv, level slog.Level, service string) *slog.Logger {
	options := &slog.HandlerOptions{
		Level: level,
		// Source locations are only worth their cost when a human reads the output.
		AddSource: env == config.EnvDevelopment,
	}

	var handler slog.Handler
	if env == config.EnvDevelopment {
		handler = slog.NewTextHandler(os.Stdout, options)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, options)
	}

	return slog.New(handler).With(slog.String("service", service))
}

package config_test

import (
	"strings"
	"testing"

	"github.com/niagga/aveniq/backend/internal/platform/config"
)

// METRICS_ADDR is the address of the private scrape listener for the process.
// Its default is deliberately empty: a multi-tenant process must not expose its
// internal measurement unless an operator asks for it explicitly.

func clearedEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("METRICS_ADDR", "")
}

func TestMetricsAddrDefaultsToNoListener(t *testing.T) {
	clearedEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MetricsAddr != "" {
		t.Fatalf("MetricsAddr = %q, want an empty default (no listener)", cfg.MetricsAddr)
	}
}

func TestMetricsAddrIsAcceptedWhenWellFormed(t *testing.T) {
	clearedEnv(t)
	t.Setenv("METRICS_ADDR", "127.0.0.1:9090")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MetricsAddr != "127.0.0.1:9090" {
		t.Fatalf("MetricsAddr = %q, want 127.0.0.1:9090", cfg.MetricsAddr)
	}
}

func TestMetricsAddrIsRejectedWhenMalformed(t *testing.T) {
	clearedEnv(t)
	t.Setenv("METRICS_ADDR", "127.0.0.1")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load accepted a metrics address without a port")
	}
	if !strings.Contains(err.Error(), "METRICS_ADDR") {
		t.Fatalf("the error must name the offending variable: %v", err)
	}
}

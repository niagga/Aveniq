package redisx_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/infrastructure/redisx"
)

// These tests need a live Redis, like the Postgres tests need a database. They skip
// without TEST_REDIS_ADDR so a plain `go test ./...` stays green on a bare machine.
func rateLimitEnv(t *testing.T) (*redisx.RateLimiter, func(string) (time.Duration, error)) {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR (for example 127.0.0.1:6380) to run the rate limit tests")
	}

	ctx := context.Background()
	client, err := redisx.NewClient(ctx, addr)
	if err != nil {
		t.Fatalf("cannot reach Redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	ttl := func(key string) (time.Duration, error) {
		return client.PTTL(ctx, "ratelimit:"+key).Result()
	}
	return redisx.NewRateLimiter(client), ttl
}

func TestRateLimiterCountsAttemptsInsideAWindow(t *testing.T) {
	limiter, _ := rateLimitEnv(t)
	ctx := context.Background()
	key := "test-" + uuid.NewString()

	for want := int64(1); want <= 3; want++ {
		got, err := limiter.Hit(ctx, key, time.Minute)
		if err != nil {
			t.Fatalf("Hit %d: %v", want, err)
		}
		if got != want {
			t.Fatalf("Hit %d = %d, want %d", want, got, want)
		}
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	limiter, _ := rateLimitEnv(t)
	ctx := context.Background()

	if _, err := limiter.Hit(ctx, "test-"+uuid.NewString(), time.Minute); err != nil {
		t.Fatalf("Hit: %v", err)
	}
	got, err := limiter.Hit(ctx, "test-"+uuid.NewString(), time.Minute)
	if err != nil {
		t.Fatalf("Hit: %v", err)
	}
	if got != 1 {
		t.Fatalf("a fresh key counted %d, want 1: one client must not use up another's allowance", got)
	}
}

func TestRateLimiterArmsTheWindowOnTheFirstHitOnly(t *testing.T) {
	limiter, ttl := rateLimitEnv(t)
	ctx := context.Background()
	key := "test-" + uuid.NewString()
	window := 2 * time.Second

	if _, err := limiter.Hit(ctx, key, window); err != nil {
		t.Fatalf("first Hit: %v", err)
	}
	first, err := ttl(key)
	if err != nil {
		t.Fatalf("PTTL: %v", err)
	}
	if first <= 0 || first > window {
		t.Fatalf("PTTL after the first hit = %v, want a window of at most %v", first, window)
	}

	time.Sleep(300 * time.Millisecond)
	if _, err := limiter.Hit(ctx, key, window); err != nil {
		t.Fatalf("second Hit: %v", err)
	}
	second, err := ttl(key)
	if err != nil {
		t.Fatalf("PTTL: %v", err)
	}
	if second >= first {
		t.Fatalf("PTTL grew from %v to %v: attempts must not extend the window", first, second)
	}
}

func TestRateLimiterStartsOverAfterTheWindow(t *testing.T) {
	limiter, _ := rateLimitEnv(t)
	ctx := context.Background()
	key := "test-" + uuid.NewString()
	window := 300 * time.Millisecond

	for range 2 {
		if _, err := limiter.Hit(ctx, key, window); err != nil {
			t.Fatalf("Hit: %v", err)
		}
	}
	time.Sleep(window + 200*time.Millisecond)

	got, err := limiter.Hit(ctx, key, window)
	if err != nil {
		t.Fatalf("Hit after the window: %v", err)
	}
	if got != 1 {
		t.Fatalf("count after the window = %d, want 1", got)
	}
}

func TestRateLimiterRefusesMalformedCalls(t *testing.T) {
	limiter, _ := rateLimitEnv(t)
	ctx := context.Background()

	if _, err := limiter.Hit(ctx, "", time.Minute); err == nil {
		t.Fatal("an empty key must be refused")
	}
	if _, err := limiter.Hit(ctx, "k", 0); err == nil {
		t.Fatal("a zero window must be refused: it would never expire or never count")
	}
}

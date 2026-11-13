package redisx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// hitScript counts an attempt and arms the window's expiry on the first one only.
// Doing both in one script matters: a crash between two separate commands would
// leave a counter with no expiry, which blocks the key forever. Arming on the first
// hit also means continued attempts do not push the window out.
var hitScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// rateLimitPrefix namespaces the counters so they never collide with other keys
// the same Redis holds (the asynq queues share this instance).
const rateLimitPrefix = "ratelimit:"

// RateLimiter is the Redis adapter for port.RateLimiter. Counters are shared by
// every API instance that points at the same Redis.
type RateLimiter struct {
	client *redis.Client
}

// NewRateLimiter builds the adapter over an existing client.
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// Hit records one attempt for key and returns the count inside the current window.
func (l *RateLimiter) Hit(ctx context.Context, key string, window time.Duration) (int64, error) {
	if key == "" {
		return 0, errors.New("redisx: rate limit key must not be empty")
	}
	if window <= 0 {
		return 0, errors.New("redisx: rate limit window must be positive")
	}

	count, err := hitScript.Run(ctx, l.client, []string{rateLimitPrefix + key}, window.Milliseconds()).Int64()
	if err != nil {
		return 0, fmt.Errorf("redisx: rate limit hit: %w", err)
	}
	return count, nil
}

// Package redisx owns the Redis adapter: connection building and the health
// probe exposed through the readiness endpoint.
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const pingTimeout = 5 * time.Second

// NewClient connects to Redis and verifies the connection before returning.
func NewClient(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
		// The local stack has no password; anything else must set one, and the
		// loader will require it once REDIS_PASSWORD exists in config.
		DB: 0,
	})

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: cannot reach %s: %w", addr, err)
	}

	return client, nil
}

// Package rediscache owns the Redis client shared by anything that needs
// Redis's role as CACHE ONLY (CLAUDE.md STACK: "cache, rate limits, sticky
// CACHE only, postback dedup") — never a system of record. Every caller
// pairs it with a durable Postgres/ClickHouse store as the actual source of
// truth, so a Redis flush degrades performance, never correctness.
package rediscache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewClient parses a redis:// URL (matching .env.example's REDIS_URL) and
// verifies connectivity, mirroring internal/postgres.NewPool.
func NewClient(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing redis url: %w", err)
	}

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pinging redis: %w", err)
	}
	return client, nil
}

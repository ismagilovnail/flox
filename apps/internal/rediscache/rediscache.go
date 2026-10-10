// Package rediscache owns the Redis client shared by anything that needs
// Redis's role as CACHE ONLY (CLAUDE.md STACK: "cache, rate limits, sticky
// CACHE only, postback dedup") — never a system of record. Every caller
// pairs it with a durable Postgres/ClickHouse store as the actual source of
// truth, so a Redis flush degrades performance, never correctness.
package rediscache

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

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

// CheckEviction logs a warning when the server would refuse writes once it
// is full. Every key FLOX writes expires, but the conversion LastStatus
// cache lives 30 days (~100 bytes per converted click, about 1 GB at 10M
// conversions a month — tech audit R2). Under Redis's default
// `noeviction`, or with no `maxmemory` at all, a full server fails every
// write: the cache only falls through to Postgres, but the rate limiter
// fails open, so the /auth limits silently stop. With `allkeys-lru` the
// oldest cache entries go first and the live counters stay.
//
// Managed Redis often disables CONFIG; then the policy cannot be read and
// is only logged as unverified — never a startup failure, Redis is cache
// only.
func CheckEviction(ctx context.Context, client *redis.Client, logger *slog.Logger) {
	cfg, err := client.ConfigGet(ctx, "maxmemory*").Result()
	if err != nil {
		logger.Info("redis eviction policy not verified (CONFIG unavailable); production needs maxmemory and allkeys-lru", "error", err)
		return
	}
	maxmemory, _ := strconv.ParseInt(cfg["maxmemory"], 10, 64)
	if problem := evictionProblem(cfg["maxmemory-policy"], maxmemory); problem != "" {
		logger.Warn("redis will refuse writes when full: "+problem+"; set maxmemory and maxmemory-policy allkeys-lru",
			"maxmemory", maxmemory, "maxmemory_policy", cfg["maxmemory-policy"])
	}
}

// evictionProblem says what is wrong with a server's memory settings, or
// "" when they are fine. Any evicting policy will do: every FLOX key has a
// TTL, so the volatile-* policies reach all of them too.
func evictionProblem(policy string, maxmemory int64) string {
	switch {
	case policy == "" || policy == "noeviction":
		return "maxmemory-policy is noeviction"
	case maxmemory == 0:
		return "maxmemory is not set"
	}
	return ""
}

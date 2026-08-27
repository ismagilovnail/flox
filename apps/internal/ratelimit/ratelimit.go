// Package ratelimit is §54's abuse-throttling defense: a fixed-window
// counter backed by Redis (INCR, then EXPIRE once on the window's first
// request) — exactly the "cache, rate limits..." role CLAUDE.md's own
// STACK section already names for Redis.
//
// Fails OPEN on a Redis error: allows the request through and logs a
// warning, rather than rejecting it. Same "a cache being down degrades
// protection, never availability" stance every other Redis-optional path
// in this codebase already takes (apps/internal/conversion.RedisStore
// falls through to Postgres directly; apps/tracker's own sticky lookups
// do the same) — a limiter that failed CLOSED would turn a Redis outage
// into a full API outage, a strictly worse failure mode than temporarily
// losing brute-force protection.
//
// Deliberately not used on apps/tracker's redirect hot path (CLAUDE.md
// non-negotiable #9: tracking p50 < 20ms/p95 < 50ms) — a synchronous
// Redis round trip on every click would blow that budget outright.
// Tracker's own click volume needs infrastructure-level protection (a
// CDN or load balancer), not an in-process limiter; see docs/security.md.
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

type Limiter struct {
	rdb    *redis.Client
	logger *slog.Logger
}

// rdb may be nil — every method degrades to "always allow," the same
// outcome as a live Redis error, so a caller that couldn't connect Redis
// at startup (best-effort everywhere else in this codebase) can still
// construct a Limiter unconditionally instead of branching.
func New(rdb *redis.Client, logger *slog.Logger) *Limiter {
	return &Limiter{rdb: rdb, logger: logger}
}

// Allow reports whether one more request under key is allowed within the
// current fixed window. The classic INCR-then-set-TTL-once pattern: the
// window's first request creates the key and sets its expiry; every
// request after that is a plain increment, so a sustained attacker costs
// one Redis round trip per request, never a growing key set.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if l.rdb == nil {
		return true, nil
	}
	n, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return true, fmt.Errorf("ratelimit: incrementing %q: %w", key, err)
	}
	if n == 1 {
		if err := l.rdb.Expire(ctx, key, window).Err(); err != nil {
			return true, fmt.Errorf("ratelimit: setting expiry for %q: %w", key, err)
		}
	}
	return n <= int64(limit), nil
}

// Middleware wraps a handler with an Allow check keyed by keyPrefix +
// keyFunc(r) — e.g. a route-scoped prefix plus the client IP, so the same
// Limiter/Redis instance can back independently-limited routes without
// their counters colliding. A Redis error is logged and the request is
// allowed through (see package doc); a 429 is written, and next is never
// called, only when Allow genuinely reports the limit exceeded.
func (l *Limiter) Middleware(keyPrefix string, keyFunc func(r *http.Request) string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "ratelimit:" + keyPrefix + ":" + keyFunc(r)
			allowed, err := l.Allow(r.Context(), key, limit, window)
			if err != nil {
				l.logger.Warn("rate limit check failed, allowing request", "error", err, "key_prefix", keyPrefix)
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				apierror.Write(w, l.logger, apierror.TooManyRequests("too many requests, try again later"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP extracts a request's client IP for use as (part of) a rate-
// limit key. Prefers X-Forwarded-For's first entry — apps/api sits behind
// a load balancer/CDN in any real deployment, the identical reasoning
// apps/tracker's own clientIP helper already documents — falling back to
// RemoteAddr for a direct connection (e.g. local dev).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

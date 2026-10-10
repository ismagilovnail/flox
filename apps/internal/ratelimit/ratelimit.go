// Package ratelimit is §54's abuse-throttling defense: a fixed-window
// counter backed by Redis (one Lua script: INCR, and the window's expiry
// when the key has none) — exactly the "cache, rate limits..." role CLAUDE.md's own
// STACK section already names for Redis.
//
// Fails OPEN on a Redis error: allows the request through and logs a
// warning, rather than rejecting it — except StrictMiddleware (/auth/*),
// which falls back to counting in process. Same "a cache being down degrades
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
// Its postback endpoint uses Failures, which limits refusals only.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
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

// allowScript counts one request and makes sure the key expires, in one
// atomic step (tech audit API1). As two calls, a lost EXPIRE — a dropped
// connection, a restart between them — leaves a key with no TTL, and that
// IP or key gets 429 for good. The TTL check also heals such a key.
var allowScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 or redis.call('PTTL', KEYS[1]) == -1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// Allow reports whether one more request under key is allowed within the
// current fixed window: the window's first request creates the key with
// its expiry, every later one is a plain increment — one Redis round trip
// per request, never a growing key set.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	if l.rdb == nil {
		return true, nil
	}
	n, err := allowScript.Run(ctx, l.rdb, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return true, fmt.Errorf("ratelimit: counting %q: %w", key, err)
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
	return l.middleware(keyPrefix, keyFunc, limit, window, nil)
}

// StrictMiddleware is Middleware for routes that must stay limited while
// Redis is down — /auth/*, where failing open meant unlimited password
// guessing for the outage (tech audit API2). On a Redis error
// it counts in this process instead: per replica, so N replicas allow up
// to N× the limit, but bounded, and still no outage of its own.
func (l *Limiter) StrictMiddleware(keyPrefix string, keyFunc func(r *http.Request) string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return l.middleware(keyPrefix, keyFunc, limit, window, newLocalCounter())
}

func (l *Limiter) middleware(keyPrefix string, keyFunc func(r *http.Request) string, limit int, window time.Duration, fallback *localCounter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "ratelimit:" + keyPrefix + ":" + keyFunc(r)
			allowed, err := l.Allow(r.Context(), key, limit, window)
			if err != nil {
				metrics.RedisErrorsTotal.WithLabelValues("ratelimit").Inc()
				if fallback == nil {
					l.logger.Warn("rate limit check failed, allowing request", "error", err, "key_prefix", keyPrefix)
					next.ServeHTTP(w, r)
					return
				}
				l.logger.Warn("rate limit check failed, counting in process", "error", err, "key_prefix", keyPrefix)
				allowed = fallback.allow(key, limit, window, time.Now())
			}
			if !allowed {
				apierror.Write(w, l.logger, apierror.TooManyRequests("too many requests, try again later"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// localCounter is StrictMiddleware's in-process fixed window, used only
// while Redis fails.
type localCounter struct {
	mu        sync.Mutex
	windows   map[string]localWindow
	lastSweep time.Time
}

type localWindow struct {
	count int
	ends  time.Time
}

func newLocalCounter() *localCounter {
	return &localCounter{windows: map[string]localWindow{}}
}

func (c *localCounter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Over windows go once per window, so keys from rotating IPs cannot
	// pile up for the length of an outage.
	if now.Sub(c.lastSweep) >= window {
		for k, w := range c.windows {
			if !now.Before(w.ends) {
				delete(c.windows, k)
			}
		}
		c.lastSweep = now
	}
	w, ok := c.windows[key]
	if !ok || !now.Before(w.ends) {
		w = localWindow{ends: now.Add(window)}
	}
	w.count++
	c.windows[key] = w
	return w.count <= limit
}

// Failures limits failed attempts rather than requests: Blocked is a read,
// only Record counts. For endpoints where legitimate volume has no bound
// worth enforcing but failures do — a network's nightly postback replay
// can be thousands of requests a minute from one IP, all valid, while a
// scan of network IDs or secrets is all failures (tech audit P4). Like
// StrictMiddleware it counts in process while Redis fails (or
// was never connected), so the limit holds per replica through an outage.
type Failures struct {
	l      *Limiter
	prefix string
	limit  int
	window time.Duration
	local  *localCounter
}

// Failures blocks a key once it has limit failures within window; the
// window starts at the key's first failure.
func (l *Limiter) Failures(keyPrefix string, limit int, window time.Duration) *Failures {
	return &Failures{l: l, prefix: "ratelimit:" + keyPrefix + ":", limit: limit, window: window, local: newLocalCounter()}
}

// Blocked reports whether key has used up its failures for the window.
func (f *Failures) Blocked(ctx context.Context, key string) bool {
	key = f.prefix + key
	if f.l.rdb != nil {
		n, err := f.l.rdb.Get(ctx, key).Int()
		switch {
		case err == nil:
			return n >= f.limit
		case errors.Is(err, redis.Nil):
			return false
		}
		metrics.RedisErrorsTotal.WithLabelValues("ratelimit").Inc()
		f.l.logger.Warn("failure limit check failed, reading in-process count", "error", err, "key_prefix", f.prefix)
	}
	return f.local.count(key, time.Now()) >= f.limit
}

// Record counts one failure for key.
func (f *Failures) Record(ctx context.Context, key string) {
	key = f.prefix + key
	if f.l.rdb != nil {
		err := allowScript.Run(ctx, f.l.rdb, []string{key}, f.window.Milliseconds()).Err()
		if err == nil {
			return
		}
		metrics.RedisErrorsTotal.WithLabelValues("ratelimit").Inc()
		f.l.logger.Warn("failure limit record failed, counting in process", "error", err, "key_prefix", f.prefix)
	}
	f.local.allow(key, f.limit, f.window, time.Now())
}

// count is key's count in its current window, without adding to it.
func (c *localCounter) count(key string, now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.windows[key]
	if !ok || !now.Before(w.ends) {
		return 0
	}
	return w.count
}

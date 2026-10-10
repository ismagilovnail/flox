package ratelimit_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
)

func mustClient(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Skip("REDIS_URL not set, skipping integration test")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parsing REDIS_URL: %v", err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// uniqueKey avoids collisions between test runs sharing one real Redis
// instance — same reasoning every other package's own uniqueEmail/seedOrg
// test helpers already use for Postgres fixtures.
func uniqueKey(t *testing.T, rdb *redis.Client) string {
	t.Helper()
	key := "test:" + idgen.New()
	t.Cleanup(func() { _ = rdb.Del(context.Background(), key).Err() })
	return key
}

func TestAllowWithinAndOverLimit(t *testing.T) {
	rdb := mustClient(t)
	ctx := context.Background()
	limiter := ratelimit.New(rdb, discardLogger())
	key := uniqueKey(t, rdb)

	for i := 1; i <= 3; i++ {
		allowed, err := limiter.Allow(ctx, key, 3, time.Minute)
		if err != nil {
			t.Fatalf("Allow #%d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("Allow #%d = false, want true (limit is 3)", i)
		}
	}

	allowed, err := limiter.Allow(ctx, key, 3, time.Minute)
	if err != nil {
		t.Fatalf("Allow #4: %v", err)
	}
	if allowed {
		t.Fatal("Allow #4 = true, want false (4th request over a limit of 3)")
	}
}

func TestAllowResetsAfterWindow(t *testing.T) {
	rdb := mustClient(t)
	ctx := context.Background()
	limiter := ratelimit.New(rdb, discardLogger())
	key := uniqueKey(t, rdb)

	// PEXPIRE: millisecond windows work.
	const window = 300 * time.Millisecond

	if allowed, err := limiter.Allow(ctx, key, 1, window); err != nil || !allowed {
		t.Fatalf("first Allow: allowed=%v err=%v", allowed, err)
	}
	if allowed, err := limiter.Allow(ctx, key, 1, window); err != nil || allowed {
		t.Fatalf("second Allow (same window): allowed=%v err=%v, want false", allowed, err)
	}

	time.Sleep(window + 300*time.Millisecond)

	if allowed, err := limiter.Allow(ctx, key, 1, window); err != nil || !allowed {
		t.Fatalf("Allow after window expired: allowed=%v err=%v, want true", allowed, err)
	}
}

// Every counted key expires: the count and its TTL are one atomic step
// (tech audit API1 — a lost EXPIRE used to mean 429 forever).
func TestAllowAlwaysLeavesATTL(t *testing.T) {
	rdb := mustClient(t)
	ctx := context.Background()
	limiter := ratelimit.New(rdb, discardLogger())
	key := uniqueKey(t, rdb)

	for i := 0; i < 3; i++ {
		if _, err := limiter.Allow(ctx, key, 10, time.Minute); err != nil {
			t.Fatal(err)
		}
		ttl, err := rdb.PTTL(ctx, key).Result()
		if err != nil {
			t.Fatal(err)
		}
		if ttl <= 0 || ttl > time.Minute {
			t.Fatalf("after request %d PTTL = %v, want within (0, 1m]", i+1, ttl)
		}
	}
}

// A key stuck without a TTL — what the old INCR-then-EXPIRE left behind —
// gets one on its next request, so the block ends with the window.
func TestAllowHealsAKeyWithoutTTL(t *testing.T) {
	rdb := mustClient(t)
	ctx := context.Background()
	limiter := ratelimit.New(rdb, discardLogger())
	key := uniqueKey(t, rdb)
	if err := rdb.Set(ctx, key, 1000, 0).Err(); err != nil {
		t.Fatal(err)
	}

	const window = 300 * time.Millisecond
	if allowed, err := limiter.Allow(ctx, key, 5, window); err != nil || allowed {
		t.Fatalf("Allow over the stuck count: allowed=%v err=%v, want false", allowed, err)
	}
	if ttl, _ := rdb.PTTL(ctx, key).Result(); ttl <= 0 {
		t.Fatalf("PTTL = %v, want the window set", ttl)
	}
	time.Sleep(window + 200*time.Millisecond)
	if allowed, err := limiter.Allow(ctx, key, 5, window); err != nil || !allowed {
		t.Fatalf("Allow after the healed window: allowed=%v err=%v, want true", allowed, err)
	}
}

func TestAllowNilClientFailsOpen(t *testing.T) {
	limiter := ratelimit.New(nil, discardLogger())
	allowed, err := limiter.Allow(context.Background(), "whatever", 1, time.Minute)
	if err != nil {
		t.Fatalf("Allow with a nil client returned an error, want fail-open: %v", err)
	}
	if !allowed {
		t.Fatal("Allow with a nil client returned false, want fail-open (true)")
	}
}

func TestMiddlewareRejectsOverLimitAndFailsOpenOnNilClient(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	t.Run("real Redis, over limit", func(t *testing.T) {
		rdb := mustClient(t)
		limiter := ratelimit.New(rdb, discardLogger())
		keySuffix := idgen.New()
		t.Cleanup(func() {
			_ = rdb.Del(context.Background(), fmt.Sprintf("ratelimit:test-mw:%s", keySuffix)).Err()
		})

		mw := limiter.Middleware("test-mw", func(r *http.Request) string { return keySuffix }, 2, time.Minute)
		handler := mw(okHandler)

		for i := 1; i <= 2; i++ {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("request #%d status = %d, want 200", i, rec.Code)
			}
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request #3 status = %d, want 429", rec.Code)
		}
	})

	t.Run("nil client fails open", func(t *testing.T) {
		limiter := ratelimit.New(nil, discardLogger())
		mw := limiter.Middleware("test-mw-nil", func(r *http.Request) string { return "x" }, 1, time.Minute)
		handler := mw(okHandler)

		for i := 1; i <= 5; i++ {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("request #%d with a nil Redis client status = %d, want 200 (fail open)", i, rec.Code)
			}
		}
	})
}

// deadRedis is a client whose every command fails: nothing listens on
// port 1.
func deadRedis(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond, DialerRetryTimeout: time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func serve(h http.Handler, ip string) int {
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = ip + ":1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestStrictMiddlewareCountsInProcessWhileRedisIsDown(t *testing.T) {
	limiter := ratelimit.New(deadRedis(t), discardLogger())
	keyFunc := func(r *http.Request) string { return r.RemoteAddr }
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	strict := limiter.StrictMiddleware("auth", keyFunc, 3, time.Minute)(ok)
	for i := 1; i <= 3; i++ {
		if code := serve(strict, "10.0.0.1"); code != http.StatusOK {
			t.Fatalf("request #%d = %d, want 200", i, code)
		}
	}
	if code := serve(strict, "10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("4th request with Redis down = %d, want 429", code)
	}
	if code := serve(strict, "10.0.0.2"); code != http.StatusOK {
		t.Fatalf("another IP = %d, want 200 (own window)", code)
	}

	// The plain middleware still fails open.
	open := limiter.Middleware("api", keyFunc, 3, time.Minute)(ok)
	for i := 1; i <= 5; i++ {
		if code := serve(open, "10.0.0.1"); code != http.StatusOK {
			t.Fatalf("fail-open request #%d = %d, want 200", i, code)
		}
	}
}

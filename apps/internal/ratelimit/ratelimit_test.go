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

	// Redis's EXPIRE has whole-second granularity — go-redis's Expire
	// silently truncates anything under 1s up to 1s (confirmed against a
	// real server: a 500ms window actually became a 1s one), so the
	// window here has to be at least 1s for this test to mean anything.
	const window = 1100 * time.Millisecond

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

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{"X-Forwarded-For single", "203.0.113.5", "10.0.0.1:12345", "203.0.113.5"},
		{"X-Forwarded-For multiple, takes first", "203.0.113.5, 10.0.0.2", "10.0.0.1:12345", "203.0.113.5"},
		{"no X-Forwarded-For, uses RemoteAddr", "", "203.0.113.9:54321", "203.0.113.9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := ratelimit.ClientIP(req); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

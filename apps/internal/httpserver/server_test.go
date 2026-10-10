package httpserver_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/ismagilovnail/flox/apps/internal/clientip"
	"github.com/ismagilovnail/flox/apps/internal/httpserver"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/ratelimit"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

type okPinger struct{}

func (okPinger) Ping(ctx context.Context) error { return nil }

func mustRedis(t *testing.T) *redis.Client {
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

type fixedAPIKeyAuthenticator struct{ orgID, userID, scope string }

func (f fixedAPIKeyAuthenticator) Authenticate(ctx context.Context, rawKey string) (string, string, string, error) {
	return f.orgID, f.userID, f.scope, nil
}

type noSessionResolver struct{}

func (noSessionResolver) ResolveSession(ctx context.Context, token string) (string, string, error) {
	return "", "", fmt.Errorf("no session in this test")
}

// generalLimitKey mirrors apps/api/main.go's own construction exactly —
// this test exists specifically to prove that wiring, so it can't reuse
// production code (main.go's isn't exported) without duplicating it.
func generalLimitKey(clientIP *clientip.Resolver) func(r *http.Request) string {
	return func(r *http.Request) string {
		if scope, ok := tenant.APIKeyScope(r.Context()); ok && scope != "" {
			if orgID, ok := tenant.OrgID(r.Context()); ok {
				return "org:" + orgID
			}
		}
		return "ip:" + clientIP.String(r)
	}
}

// TestGeneralRateLimitKeysAPITrafficByOrgNotIP is this phase's actual
// regression target: an API-key-authenticated request must land in a
// Redis bucket keyed by its organization, not by client IP — the whole
// point of moving tenant.PeekIdentity ahead of the general limiter
// (ROADMAP.md: "General rate limit stays IP-keyed"). Verified by reading
// the real Redis key the limiter created, rather than by driving the
// fixed 300/min limit to a 429 (impractically slow for a unit test, and
// ratelimit_test.go already covers the increment/expire mechanics
// directly).
func TestGeneralRateLimitKeysAPITrafficByOrgNotIP(t *testing.T) {
	rdb := mustRedis(t)
	ctx := context.Background()
	orgID := "org-" + idgen.New()
	t.Cleanup(func() { _ = rdb.Del(ctx, "ratelimit:api:org:"+orgID).Err() })

	clientIPResolver, err := clientip.New(nil, discardLogger())
	if err != nil {
		t.Fatalf("clientip.New: %v", err)
	}

	limiter := ratelimit.New(rdb, discardLogger())
	identify := tenant.PeekIdentity(noSessionResolver{}, fixedAPIKeyAuthenticator{orgID: orgID, userID: "user1", scope: "write"}, discardLogger())
	srv := httpserver.New(discardLogger(), "test", okPinger{}, nil, "http://localhost:3000", limiter, generalLimitKey(clientIPResolver), identify)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer flox_sk_whatever")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/health status = %d, want 200", rec.Code)
	}

	// /health is itself exempt from the limiter, so it never increments a
	// key on its own — hit a route the limiter DOES cover to actually
	// create one. There's no domain route mounted on this bare Server
	// (those are added by apps/api/main.go, one layer up), so use a path
	// that 404s past the router but still passes through every global
	// middleware first, including the limiter.
	req2 := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
	req2.Header.Set("Authorization", "Bearer flox_sk_whatever")
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)

	exists, err := rdb.Exists(ctx, "ratelimit:api:org:"+orgID).Result()
	if err != nil {
		t.Fatalf("checking org-keyed Redis key: %v", err)
	}
	if exists != 1 {
		t.Fatalf("ratelimit:api:org:%s does not exist — the general limiter did not key this API-key request by organization", orgID)
	}
}

// TestGeneralRateLimitKeysUnauthenticatedTrafficByIP confirms the other
// half: a request with no valid credential at all still falls back to
// the original IP-keyed behavior, unchanged from before this phase.
func TestGeneralRateLimitKeysUnauthenticatedTrafficByIP(t *testing.T) {
	rdb := mustRedis(t)
	ctx := context.Background()

	clientIPResolver, err := clientip.New(nil, discardLogger())
	if err != nil {
		t.Fatalf("clientip.New: %v", err)
	}

	limiter := ratelimit.New(rdb, discardLogger())
	identify := tenant.PeekIdentity(noSessionResolver{}, fixedAPIKeyAuthenticator{}, discardLogger())
	srv := httpserver.New(discardLogger(), "test", okPinger{}, nil, "http://localhost:3000", limiter, generalLimitKey(clientIPResolver), identify)

	req := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
	wantKey := "ratelimit:api:ip:" + clientIPResolver.String(req)
	t.Cleanup(func() { _ = rdb.Del(ctx, wantKey).Err() })

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	exists, err := rdb.Exists(ctx, wantKey).Result()
	if err != nil {
		t.Fatalf("checking IP-keyed Redis key: %v", err)
	}
	if exists != 1 {
		t.Fatalf("%s does not exist — an unauthenticated request must still key by client IP", wantKey)
	}
}

// Every API response carries the security headers (tech audit S2); HSTS
// only when asked for — once TLS is in front of the API.
func TestSecurityHeaders(t *testing.T) {
	passThrough := func(next http.Handler) http.Handler { return next }
	limiter := ratelimit.New(nil, discardLogger())
	keyAll := func(*http.Request) string { return "all" }
	for _, hsts := range []bool{false, true} {
		srv := httpserver.New(discardLogger(), "test", okPinger{}, nil, "http://localhost:3000", limiter, keyAll, passThrough, httpserver.WithHSTS(hsts))
		srv.Mux().Get("/thing", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
		for _, path := range []string{"/health", "/v1/thing", "/no-such-route"} {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			h := rec.Header()
			if h.Get("Content-Security-Policy") != "default-src 'none'; frame-ancestors 'none'" ||
				h.Get("X-Frame-Options") != "DENY" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" {
				t.Errorf("hsts=%v %s: headers %v", hsts, path, h)
			}
			if got := h.Get("Strict-Transport-Security") != ""; got != hsts {
				t.Errorf("hsts=%v %s: Strict-Transport-Security present = %v", hsts, path, got)
			}
		}
	}
}

// Domain routes are served under /v1 only; health, readiness and metrics
// stay at the root for probes and scrapers (tech audit API3).
func TestDomainRoutesLiveUnderV1(t *testing.T) {
	passThrough := func(next http.Handler) http.Handler { return next }
	srv := httpserver.New(discardLogger(), "test", okPinger{}, nil, "http://localhost:3000",
		ratelimit.New(nil, discardLogger()), func(*http.Request) string { return "all" }, passThrough)
	srv.Mux().Get("/thing", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	for path, want := range map[string]int{
		"/v1/thing":  http.StatusTeapot,
		"/thing":     http.StatusNotFound,
		"/health":    http.StatusOK,
		"/ready":     http.StatusOK,
		"/metrics":   http.StatusOK,
		"/v1/health": http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

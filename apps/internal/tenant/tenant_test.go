package tenant_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

// countingResolver/countingAPIKeyAuthenticator record how many times
// they were called — used to prove NewMiddleware skips re-resolution
// when PeekIdentity already ran (below), and separately to drive
// PeekIdentity's own success/failure cases.
type countingResolver struct {
	orgID, userID string
	err           error
	calls         int
}

func (c *countingResolver) ResolveSession(ctx context.Context, token string) (string, string, error) {
	c.calls++
	if c.err != nil {
		return "", "", c.err
	}
	return c.orgID, c.userID, nil
}

type countingAPIKeyAuthenticator struct {
	orgID, userID, scope string
	err                  error
	calls                int
}

func (c *countingAPIKeyAuthenticator) Authenticate(ctx context.Context, rawKey string) (string, string, string, error) {
	c.calls++
	if c.err != nil {
		return "", "", "", c.err
	}
	return c.orgID, c.userID, c.scope, nil
}

func TestPeekIdentityResolvesAPIKey(t *testing.T) {
	apiKeys := &countingAPIKeyAuthenticator{orgID: "org1", userID: "user1", scope: "write"}
	resolver := &countingResolver{}
	var gotOrgID, gotUserID, gotScope string
	var gotScopeOK bool
	handler := tenant.PeekIdentity(resolver, apiKeys, discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrgID, _ = tenant.OrgID(r.Context())
		gotUserID, _ = tenant.UserID(r.Context())
		gotScope, gotScopeOK = tenant.APIKeyScope(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
	req.Header.Set("Authorization", "Bearer flox_sk_whatever")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (PeekIdentity never rejects)", rec.Code)
	}
	if gotOrgID != "org1" || gotUserID != "user1" || !gotScopeOK || gotScope != "write" {
		t.Fatalf("context = (org=%q user=%q scope=%q ok=%v), want (org1, user1, write, true)", gotOrgID, gotUserID, gotScope, gotScopeOK)
	}
	if apiKeys.calls != 1 || resolver.calls != 0 {
		t.Fatalf("apiKeys.calls=%d resolver.calls=%d, want (1, 0) — a Bearer token must never also check the cookie", apiKeys.calls, resolver.calls)
	}
}

func TestPeekIdentityResolvesSessionCookie(t *testing.T) {
	resolver := &countingResolver{orgID: "org2", userID: "user2"}
	apiKeys := &countingAPIKeyAuthenticator{}
	var gotOrgID, gotUserID string
	var gotScopeOK bool
	handler := tenant.PeekIdentity(resolver, apiKeys, discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrgID, _ = tenant.OrgID(r.Context())
		gotUserID, _ = tenant.UserID(r.Context())
		_, gotScopeOK = tenant.APIKeyScope(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
	req.AddCookie(&http.Cookie{Name: tenant.CookieName, Value: "irrelevant-to-the-fake-resolver"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotOrgID != "org2" || gotUserID != "user2" || gotScopeOK {
		t.Fatalf("context = (org=%q user=%q apiKeyScopeSet=%v), want (org2, user2, false)", gotOrgID, gotUserID, gotScopeOK)
	}
	if resolver.calls != 1 || apiKeys.calls != 0 {
		t.Fatalf("resolver.calls=%d apiKeys.calls=%d, want (1, 0)", resolver.calls, apiKeys.calls)
	}
}

// TestPeekIdentityNeverRejects is the core contract that lets this
// middleware be mounted globally, ahead of every route including
// genuinely public ones (/health, /auth/login, webhooks) — an invalid
// or entirely absent credential must never produce anything but a plain
// 200 from downstream, unlike NewMiddleware's own 401.
func TestPeekIdentityNeverRejects(t *testing.T) {
	cases := []struct {
		name       string
		setRequest func(r *http.Request)
	}{
		{"no credential at all", func(r *http.Request) {}},
		{"invalid bearer token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer bad-key") }},
		{"invalid session cookie", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: tenant.CookieName, Value: "expired-or-unknown"})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &countingResolver{err: apierror.Unauthorized("session not found")}
			apiKeys := &countingAPIKeyAuthenticator{err: apierror.Unauthorized("invalid api key")}
			var gotOrgID string
			var gotOK bool
			handler := tenant.PeekIdentity(resolver, apiKeys, discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotOrgID, gotOK = tenant.OrgID(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
			tc.setRequest(req)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 — PeekIdentity must never reject", rec.Code)
			}
			if gotOK {
				t.Fatalf("OrgID = %q, ok=true, want unset — a failed resolution must not leave stale identity in context", gotOrgID)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("PeekIdentity set/cleared a cookie — that's NewMiddleware's job on a genuinely protected route, not this best-effort pass")
			}
		})
	}
}

// TestNewMiddlewareSkipsReresolutionWhenAlreadyIdentified is the other
// half of the general-rate-limiter fix: PeekIdentity (mounted globally,
// ahead of every route) and NewMiddleware (mounted per protected route
// group) must resolve identity at most ONCE per request between them,
// not twice — otherwise every authenticated apps/api request would pay
// for a second Postgres/session round trip for no reason.
func TestNewMiddlewareSkipsReresolutionWhenAlreadyIdentified(t *testing.T) {
	peekAPIKeys := &countingAPIKeyAuthenticator{orgID: "org1", userID: "user1", scope: "write"}
	peekResolver := &countingResolver{}
	identify := tenant.PeekIdentity(peekResolver, peekAPIKeys, discardLogger())

	// If NewMiddleware re-resolves instead of trusting PeekIdentity's
	// result, it calls THESE — both configured to fail, so the test
	// fails loudly (401, not a silent false-pass) if that ever happens.
	reresolveAPIKeys := &countingAPIKeyAuthenticator{err: apierror.Unauthorized("must not be called")}
	reresolveResolver := &countingResolver{err: apierror.Unauthorized("must not be called")}
	protected := tenant.NewMiddleware(reresolveResolver, reresolveAPIKeys, discardLogger(), false)(okHandler())

	handler := identify(protected)

	req := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
	req.Header.Set("Authorization", "Bearer flox_sk_whatever")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — NewMiddleware should have trusted PeekIdentity's already-resolved context", rec.Code)
	}
	if reresolveAPIKeys.calls != 0 || reresolveResolver.calls != 0 {
		t.Fatalf("NewMiddleware re-resolved identity (apiKeys.calls=%d resolver.calls=%d), want (0, 0) — it should skip when OrgID is already in context",
			reresolveAPIKeys.calls, reresolveResolver.calls)
	}
	if peekAPIKeys.calls != 1 {
		t.Fatalf("PeekIdentity's own apiKeys.calls = %d, want 1", peekAPIKeys.calls)
	}
}

// TestNewMiddlewareStillEnforcesWhenNotPreIdentified confirms the
// short-circuit didn't quietly turn NewMiddleware into a no-op auth
// check — a request PeekIdentity never saw (or couldn't identify) must
// still be rejected exactly as before this phase.
func TestNewMiddlewareStillEnforcesWhenNotPreIdentified(t *testing.T) {
	resolver := &countingResolver{err: apierror.Unauthorized("session not found")}
	apiKeys := &countingAPIKeyAuthenticator{}
	protected := tenant.NewMiddleware(resolver, apiKeys, discardLogger(), false)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/campaigns", nil)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a request with no cookie and no API key", rec.Code)
	}
}

func TestRequireSameOrigin(t *testing.T) {
	mw := tenant.RequireSameOrigin("https://app.floxlink.io", discardLogger())
	handler := mw(okHandler())

	cases := []struct {
		name       string
		method     string
		origin     string
		wantStatus int
	}{
		{"matching origin allowed", http.MethodPost, "https://app.floxlink.io", http.StatusOK},
		{"mismatched origin rejected", http.MethodPost, "https://evil.example", http.StatusForbidden},
		{"missing origin allowed (non-browser client)", http.MethodPost, "", http.StatusOK},
		{"GET never checked, even cross-origin", http.MethodGet, "https://evil.example", http.StatusOK},
		{"PATCH mismatched origin rejected", http.MethodPatch, "https://evil.example", http.StatusForbidden},
		{"DELETE mismatched origin rejected", http.MethodDelete, "https://evil.example", http.StatusForbidden},
		{"PUT mismatched origin rejected", http.MethodPut, "https://evil.example", http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/campaigns", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

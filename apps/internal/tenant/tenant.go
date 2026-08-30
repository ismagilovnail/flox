// Package tenant carries the current request's organization_id (§36-
// TENANCY) and authenticated user_id through request context.
//
// Phase 28 (§52) replaced this package's original X-Organization-Id-header
// stand-in with real session-cookie authentication — SessionResolver is
// implemented by apps/internal/auth.Service, injected via NewMiddleware
// from apps/api/main.go. This package deliberately does NOT import auth
// (auth imports tenant, for tenant.OrgID/UserID in its own handlers/
// permission middleware); SessionResolver is declared here, at the point
// of use, so main.go can wire a *auth.Service into it with no import
// cycle. Nothing downstream of this middleware changed: every handler
// still only ever sees organization_id via OrgID(ctx), exactly as before
// — a handler still physically cannot pull it from anywhere else, which is
// the actual property §36-TENANCY's "never from the request body" is
// protecting.
package tenant

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
)

// mutatingMethods is what RequireSameOrigin gates — GET/HEAD/OPTIONS never
// change state, so there is nothing for CSRF to forge.
var mutatingMethods = map[string]bool{
	http.MethodPost:   true,
	http.MethodPatch:  true,
	http.MethodPut:    true,
	http.MethodDelete: true,
}

// CookieName is the session cookie apps/internal/auth sets on
// signup/login/accept-invite and clears on logout.
const CookieName = "flox_session"

type contextKey struct{}
type userContextKey struct{}

// SessionResolver validates a raw session token (the cookie's value) and
// returns the organization/user it belongs to. Implemented by
// apps/internal/auth.Service — a session token that doesn't exist, or has
// expired, must return an *apierror.Error (apierror.Unauthorized), not a
// bare error, so the middleware can distinguish "not logged in" (401) from
// a genuine server fault (500).
type SessionResolver interface {
	ResolveSession(ctx context.Context, token string) (organizationID, userID string, err error)
}

// NewMiddleware builds the chi-compatible middleware every tenant-scoped
// route mounts. A request with no session cookie, or one ResolveSession
// rejects, never reaches a handler.
//
// secureCookies must match the Secure attribute auth.Handler used when it
// set the cookie (false in local dev, true anywhere served over HTTPS) —
// clearing a cookie is itself a Set-Cookie, and a mismatched Secure flag
// means the browser won't recognize it as the same cookie to overwrite.
func NewMiddleware(resolver SessionResolver, logger *slog.Logger, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				apierror.Write(w, logger, apierror.Unauthorized("not signed in"))
				return
			}

			orgID, userID, err := resolver.ResolveSession(r.Context(), cookie.Value)
			if err != nil {
				// A present-but-invalid cookie (expired, revoked, or its
				// session row gone) left uncleared would keep proxy.ts's
				// cookie-*presence* check convinced the visitor is signed
				// in forever: it bounces /login back to /overview, whose
				// AuthGuard 401s here and bounces back to /login — an
				// infinite redirect loop with no error surfaced anywhere,
				// which is exactly what a stale dev session produced
				// (2026-08-30, reported as a black screen with nothing
				// loading). Clearing it here means the very next hit
				// breaks the loop on its own, no manual cookie-clear
				// needed.
				clearSessionCookie(w, secureCookies)
				apierror.Write(w, logger, err)
				return
			}

			ctx := context.WithValue(r.Context(), contextKey{}, orgID)
			ctx = context.WithValue(ctx, userContextKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// clearSessionCookie mirrors apps/internal/auth.Handler's own (unexported)
// clearSessionCookie — duplicated rather than imported because tenant must
// not import auth (see the package doc comment on the import-cycle this
// avoids). Keep the two in sync if either cookie's attributes change.
func clearSessionCookie(w http.ResponseWriter, secureCookies bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// RequireSameOrigin is §54's CSRF defense: SameSite=Lax on the session
// cookie (apps/internal/auth's own doc comment on setSessionCookie)
// already blocks the classic cross-site-form-POST CSRF, but not a
// cross-site fetch/XHR issued with credentials:"include" — Lax still
// attaches the cookie to those in some browsers/configurations. This
// closes that gap with the modern, simpler alternative to a double-
// submit CSRF token: reject any mutating (POST/PATCH/PUT/DELETE) request
// whose Origin header is PRESENT and does not match appURL exactly.
//
// Deliberately does NOT reject a request with no Origin header at all —
// browsers always send Origin on a cross-origin fetch/XHR and on same-
// origin state-changing requests per the Fetch spec, so an absent Origin
// means a non-browser client (curl, an operator's own script, a health
// check) rather than a browser silently omitting it; rejecting those
// too would break legitimate API access for no CSRF benefit (a non-
// browser client was never subject to SameSite/CORS in the first place).
//
// Mount before (outside) the session-resolving middleware — rejecting a
// forged cross-origin request is cheaper and safer to do before ever
// looking up whose session it claims to be.
func RequireSameOrigin(appURL string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mutatingMethods[r.Method] {
				if origin := r.Header.Get("Origin"); origin != "" && origin != appURL {
					apierror.Write(w, logger, apierror.Forbidden("cross-origin request rejected"))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OrgID returns the current request's organization id. Only ever called
// after the tenant middleware has run, so the zero value/ok=false case is a
// programmer error (route mounted outside the middleware chain), not a
// normal runtime condition.
func OrgID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(contextKey{}).(string)
	return v, ok
}

// UserID returns the current request's authenticated user id. Same
// programmer-error contract as OrgID.
func UserID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userContextKey{}).(string)
	return v, ok
}

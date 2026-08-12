// Package tenant carries the current request's organization_id (§36-
// TENANCY) through request context.
//
// There is no session/API-key auth yet — that's Phase 28. Until then, the
// organization_id comes from an X-Organization-Id header, which the caller
// (today: manual testing, integration tests; eventually: Phase 27's
// frontend integration) must set to a real, already-existing organization
// id. This is a deliberate, temporary stand-in, not a shortcut: it still
// satisfies §36-TENANCY's literal requirement ("never from the request
// body") since the header is read once, by this middleware, and every
// handler downstream only ever sees the validated context value — a
// handler physically cannot pull organization_id from anywhere else,
// which is the actual property the invariant is protecting. Phase 28
// replaces the middleware's header lookup with a session/API-key lookup;
// nothing downstream of it changes.
package tenant

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ismagilovnail/flox/apps/internal/apierror"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

const headerName = "X-Organization-Id"

type contextKey struct{}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgID := r.Header.Get(headerName)
		if orgID == "" || !idgen.IsValid(orgID) {
			apiErr := apierror.Validation("missing or invalid "+headerName+" header", nil)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(apiErr.Status)
			_ = json.NewEncoder(w).Encode(apiErr)
			return
		}
		ctx := context.WithValue(r.Context(), contextKey{}, orgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OrgID returns the current request's organization id. Only ever called
// after Middleware has run, so the zero value/ok=false case is a
// programmer error (route mounted outside the middleware chain), not a
// normal runtime condition.
func OrgID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(contextKey{}).(string)
	return v, ok
}

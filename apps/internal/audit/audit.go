// Package audit writes the organization's audit trail (§54 "audit
// logging") into audit_logs (migration 00010): one row per successful
// mutating request on a tenant route, with who did it — the signed-in user,
// or the API key and its creator — and what it touched.
//
// Middleware sits inside the tenant middleware (apps/api/main.go), so every
// tenant route is covered without per-handler code, and a route added
// later is audited from its first day. By default a row names the request
// itself: action "PATCH /campaigns/{campaign}", the entity from the last
// URL parameter (or the created resource's top-level "id" in a 201
// response). A handler or service that knows better names the action
// instead with Annotate ("api_key.rotated"); several annotations in one
// request become several rows.
//
// Request and response bodies are never stored: they carry secrets (API
// keys, postback secrets, ad-platform tokens) that §54 forbids logging.
// The row keeps only method, route pattern, status and the key id.
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

// Execer is the slice of *pgxpool.Pool the middleware writes through.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Entry names what a request did, for Annotate.
type Entry struct {
	Action     string // e.g. "api_key.rotated"
	EntityType string // e.g. "api_key"
	EntityID   string
}

type recorderKey struct{}

type recorder struct {
	mu      sync.Mutex
	entries []Entry
}

// Annotate names the current request's action (or one of them). It
// reports false when ctx carries no audit middleware — a pre-session route
// such as accepting an invite — and the caller then writes its own row.
func Annotate(ctx context.Context, e Entry) bool {
	rec, ok := ctx.Value(recorderKey{}).(*recorder)
	if !ok {
		return false
	}
	rec.mu.Lock()
	rec.entries = append(rec.entries, e)
	rec.mu.Unlock()
	return true
}

var mutating = map[string]bool{
	http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
}

// maxCapturedBody bounds how much of a 201 response is kept to read the
// created resource's id from; a bigger body is not parsed.
const maxCapturedBody = 64 << 10

// Middleware records every 2xx POST/PUT/PATCH/DELETE whose "METHOD
// pattern" is not in skip (POST-shaped reads such as the analytics
// queries). Patterns are read without mountPrefix, the prefix the router
// is mounted under ("/v1"), so they match the routes as registered. It
// must run after the tenant middleware has put the
// organization into the context; a request without one is not recorded.
// Writing the row is best-effort: a failed insert is logged, the
// response has already been sent.
func Middleware(db Execer, skip map[string]bool, mountPrefix string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !mutating[r.Method] {
				next.ServeHTTP(w, r)
				return
			}
			rec := &recorder{}
			ctx := context.WithValue(r.Context(), recorderKey{}, rec)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			body := &capped{limit: maxCapturedBody}
			ww.Tee(body)
			next.ServeHTTP(ww, r.WithContext(ctx))

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			if status < 200 || status >= 300 {
				return
			}
			pattern := routePattern(r, mountPrefix)
			if skip[r.Method+" "+pattern] {
				return
			}
			orgID, ok := tenant.OrgID(ctx)
			if !ok || orgID == "" {
				return
			}

			entries := rec.entries
			if len(entries) == 0 {
				entries = []Entry{defaultEntry(r, pattern, status, body)}
			}
			var actor *string
			if userID, ok := tenant.UserID(ctx); ok && userID != "" {
				actor = &userID
			}
			meta := map[string]any{"method": r.Method, "route": pattern, "status": status}
			if keyID, ok := tenant.APIKeyID(ctx); ok {
				meta["apiKeyId"] = keyID
			}
			metaJSON, _ := json.Marshal(meta)

			writeCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			for _, e := range entries {
				if _, err := db.Exec(writeCtx,
					`INSERT INTO audit_logs (id, organization_id, actor_user_id, action, entity_type, entity_id, metadata)
					 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
					idgen.New(), orgID, actor, e.Action, e.EntityType, e.EntityID, metaJSON,
				); err != nil {
					logger.Error("audit: writing audit log", "error", err, "action", e.Action, "organization_id", orgID)
				}
			}
		})
	}
}

// routePattern is the matched chi pattern without mountPrefix and without
// a trailing slash (a route mounted at a group's root reads "/campaigns/").
func routePattern(r *http.Request, mountPrefix string) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return r.URL.Path
	}
	p := strings.TrimPrefix(rctx.RoutePattern(), mountPrefix)
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// defaultEntry names a request no one annotated: the action is the method
// and route; the entity is the last URL parameter, typed by the path
// segment before it ("/campaigns/{campaign}/stream-sets/{streamSet}" →
// stream-sets), or for a create with no parameter the response's "id".
func defaultEntry(r *http.Request, pattern string, status int, body *capped) Entry {
	e := Entry{Action: r.Method + " " + pattern}
	segments := strings.Split(strings.Trim(pattern, "/"), "/")
	lastParam := -1
	for i, s := range segments {
		if strings.HasPrefix(s, "{") {
			lastParam = i
		}
	}
	if lastParam >= 0 {
		name := strings.Trim(segments[lastParam], "{}*")
		if i := strings.IndexByte(name, ':'); i >= 0 {
			name = name[:i]
		}
		e.EntityID = chi.URLParam(r, name)
		if lastParam > 0 {
			e.EntityType = segments[lastParam-1]
		}
		return e
	}
	e.EntityType = segments[len(segments)-1]
	if status == http.StatusCreated && !body.overflow {
		var created struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body.buf.Bytes(), &created) == nil {
			e.EntityID = created.ID
		}
	}
	return e
}

// capped keeps the first limit bytes written to it and notes whether more
// came.
type capped struct {
	buf      bytes.Buffer
	limit    int
	overflow bool
}

func (c *capped) Write(p []byte) (int, error) {
	if c.overflow || c.buf.Len()+len(p) > c.limit {
		c.overflow = true
		c.buf.Reset()
		return len(p), nil
	}
	return c.buf.Write(p)
}

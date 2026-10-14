package audit_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ismagilovnail/flox/apps/internal/audit"
	"github.com/ismagilovnail/flox/apps/internal/logging"
	"github.com/ismagilovnail/flox/apps/internal/tenant"
)

type row struct {
	orgID, action, entityType, entityID string
	actor                               *string
	meta                                map[string]any
}

type fakeDB struct {
	mu   sync.Mutex
	rows []row
}

func (f *fakeDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var meta map[string]any
	_ = json.Unmarshal(args[6].([]byte), &meta)
	f.rows = append(f.rows, row{
		orgID: args[1].(string), actor: args[2].(*string), action: args[3].(string),
		entityType: args[4].(string), entityID: args[5].(string), meta: meta,
	})
	return pgconn.CommandTag{}, nil
}

type sessions struct{}

func (sessions) ResolveSession(context.Context, string) (string, string, error) {
	return "org-1", "user-1", nil
}

type apiKeys struct{}

func (apiKeys) Authenticate(context.Context, string) (tenant.APIKeyIdentity, error) {
	return tenant.APIKeyIdentity{KeyID: "key-9", OrganizationID: "org-1", UserID: "user-2", Scope: "write"}, nil
}

// router mounts the routes the tests call behind the real tenant
// middleware, with the audit middleware inside it as apps/api does.
func router(db *fakeDB, skip map[string]bool) http.Handler {
	logger := logging.New("error")
	session := tenant.NewMiddleware(sessions{}, apiKeys{}, logger, false)
	trail := audit.Middleware(db, skip, "", logger)
	r := chi.NewRouter()
	r.Route("/campaigns", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler { return session(trail(next)) })
		r.Post("/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"camp-new","name":"x"}`))
		})
		r.Get("/{campaign}", func(w http.ResponseWriter, _ *http.Request) {})
		r.Patch("/{campaign}", func(w http.ResponseWriter, _ *http.Request) {})
		r.Delete("/{campaign}/stream-sets/{streamSet}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		r.Post("/{campaign}/fail", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
		})
		r.Post("/{campaign}/query", func(w http.ResponseWriter, _ *http.Request) {})
		r.Post("/{campaign}/two-things", func(w http.ResponseWriter, r *http.Request) {
			audit.Annotate(r.Context(), audit.Entry{Action: "team.role_changed", EntityType: "membership", EntityID: "m1"})
			audit.Annotate(r.Context(), audit.Entry{Action: "team.suspended", EntityType: "membership", EntityID: "m1"})
		})
	})
	return r
}

func do(t *testing.T, h http.Handler, method, path string, bearer bool) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{"secret":"never-stored"}`))
	if bearer {
		req.Header.Set("Authorization", "Bearer flx_live_x")
	} else {
		req.AddCookie(&http.Cookie{Name: tenant.CookieName, Value: "s"})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestMiddlewareRecordsSuccessfulWrites(t *testing.T) {
	db := &fakeDB{}
	h := router(db, nil)

	do(t, h, http.MethodPatch, "/campaigns/c1", false)
	do(t, h, http.MethodPost, "/campaigns", false)
	do(t, h, http.MethodDelete, "/campaigns/c1/stream-sets/s1", true)

	if len(db.rows) != 3 {
		t.Fatalf("rows = %d, want 3: %+v", len(db.rows), db.rows)
	}
	patch, create, del := db.rows[0], db.rows[1], db.rows[2]
	if patch.action != "PATCH /campaigns/{campaign}" || patch.entityType != "campaigns" || patch.entityID != "c1" {
		t.Errorf("PATCH row = %+v", patch)
	}
	if patch.orgID != "org-1" || patch.actor == nil || *patch.actor != "user-1" {
		t.Errorf("PATCH row org/actor = %s/%v, want org-1/user-1 from the session", patch.orgID, patch.actor)
	}
	if _, ok := patch.meta["apiKeyId"]; ok || patch.meta["route"] != "/campaigns/{campaign}" || patch.meta["status"] != float64(200) {
		t.Errorf("PATCH metadata = %v", patch.meta)
	}
	if create.action != "POST /campaigns" || create.entityType != "campaigns" || create.entityID != "camp-new" {
		t.Errorf("create row = %+v, want the created id from the 201 body", create)
	}
	if del.entityType != "stream-sets" || del.entityID != "s1" {
		t.Errorf("DELETE row = %+v, want the last URL parameter", del)
	}
	if del.meta["apiKeyId"] != "key-9" || del.actor == nil || *del.actor != "user-2" {
		t.Errorf("API key row = actor %v meta %v, want the key and its creator", del.actor, del.meta)
	}
	for _, r := range db.rows {
		if strings.Contains(r.action+r.entityID+r.entityType, "never-stored") {
			t.Fatal("request body reached the audit trail")
		}
		for _, v := range r.meta {
			if s, ok := v.(string); ok && strings.Contains(s, "never-stored") {
				t.Fatal("request body reached the audit metadata")
			}
		}
	}
}

func TestMiddlewareSkipsReadsFailuresAndSkippedRoutes(t *testing.T) {
	db := &fakeDB{}
	h := router(db, map[string]bool{"POST /campaigns/{campaign}/query": true})

	do(t, h, http.MethodGet, "/campaigns/c1", false)
	if code := do(t, h, http.MethodPost, "/campaigns/c1/fail", false); code != http.StatusUnprocessableEntity {
		t.Fatalf("fail route = %d", code)
	}
	do(t, h, http.MethodPost, "/campaigns/c1/query", false)

	if len(db.rows) != 0 {
		t.Fatalf("rows = %+v, want none (a read, a failed write, a skipped POST-shaped read)", db.rows)
	}
}

func TestAnnotationsNameTheAction(t *testing.T) {
	db := &fakeDB{}
	do(t, router(db, nil), http.MethodPost, "/campaigns/c1/two-things", false)

	if len(db.rows) != 2 || db.rows[0].action != "team.role_changed" || db.rows[1].action != "team.suspended" {
		t.Fatalf("rows = %+v, want one row per annotation, in order", db.rows)
	}
	if db.rows[0].entityType != "membership" || db.rows[0].entityID != "m1" || db.rows[0].meta["route"] != "/campaigns/{campaign}/two-things" {
		t.Fatalf("annotated row = %+v", db.rows[0])
	}
}

func TestAnnotateOutsideTheMiddleware(t *testing.T) {
	if audit.Annotate(context.Background(), audit.Entry{Action: "x"}) {
		t.Fatal("Annotate reported success with no audit middleware in the context")
	}
}

type failingDB struct{}

func (failingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("database down")
}

// A failed audit write never changes the response the client already got.
func TestMiddlewareWriteFailureKeepsTheResponse(t *testing.T) {
	logger := logging.New("error")
	session := tenant.NewMiddleware(sessions{}, apiKeys{}, logger, false)
	h := session(audit.Middleware(failingDB{}, nil, "", logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})))
	if code := do(t, h, http.MethodPost, "/anything", false); code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", code)
	}
}

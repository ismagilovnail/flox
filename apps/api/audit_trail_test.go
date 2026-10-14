package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestAuditTrail runs the real API (§54 "audit logging", "API key
// rotation"): a write through a session and one through an API key in its
// rotation grace period each leave an audit_logs row naming the actor and,
// for the key, which key; a POST-shaped read leaves none. Every auditSkip
// entry must still be a route of the real router.
func TestAuditTrail(t *testing.T) {
	env := newIsolationEnv(t)
	c, ctx, pool := env.c, env.ctx, env.pool

	routes := map[string]bool{}
	if err := chi.Walk(env.srv.Mux(), func(method, path string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method+" "+strings.TrimSuffix(path, "/")] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for r := range auditSkip {
		if !routes[r] {
			t.Errorf("auditSkip lists %q, which is not a route", r)
		}
	}

	status, body, _ := c.request(http.MethodPost, "/api-keys", map[string]string{"name": "Audit sync", "scope": "write"})
	if status != http.StatusCreated {
		t.Fatalf("create key = %d %s", status, body)
	}
	var created struct {
		ApiKey  struct{ ID string } `json:"apiKey"`
		FullKey string              `json:"fullKey"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	status, body, _ = c.request(http.MethodPost, "/api-keys/"+created.ApiKey.ID+"/rotate", nil)
	if status != http.StatusCreated {
		t.Fatalf("rotate key = %d %s", status, body)
	}
	var rotated struct {
		ApiKey  struct{ ID string } `json:"apiKey"`
		FullKey string              `json:"fullKey"`
	}
	_ = json.Unmarshal([]byte(body), &rotated)
	if rotated.FullKey == "" || rotated.FullKey == created.FullKey {
		t.Fatalf("rotate returned no new secret: %s", body)
	}

	// The old key, inside its grace period, still writes.
	req := httptest.NewRequest(http.MethodPost, "/v1/tags", strings.NewReader(`{"name":"audit-via-key","color":"blue"}`))
	req.Header.Set("Authorization", "Bearer "+created.FullKey)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "[" + c.ip + "]:1234"
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("write with the rotated key = %d %s", rec.Code, rec.Body.String())
	}

	if status, body, _ := c.request(http.MethodPost, "/analytics/report", map[string]any{"dims": []string{}, "from": "2026-10-01", "to": "2026-10-02"}); status != http.StatusOK {
		t.Fatalf("report = %d %s", status, body)
	}

	rows, err := pool.Query(ctx, `
		SELECT action, entity_type, entity_id, coalesce(actor_user_id, ''), coalesce(metadata->>'apiKeyId', ''), coalesce(metadata->>'route', '')
		FROM audit_logs WHERE organization_id = $1 ORDER BY created_at, id`, env.orgA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type entry struct{ action, entityType, entityID, actor, apiKeyID, route string }
	var got []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.action, &e.entityType, &e.entityID, &e.actor, &e.apiKeyID, &e.route); err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	find := func(action string) *entry {
		for i := range got {
			if got[i].action == action {
				return &got[i]
			}
		}
		t.Fatalf("no %q row in %+v", action, got)
		return nil
	}

	if e := find("api_key.created"); e.entityID != created.ApiKey.ID || e.actor != env.userA || e.apiKeyID != "" {
		t.Errorf("api_key.created = %+v", e)
	}
	if e := find("api_key.rotated"); e.entityID != created.ApiKey.ID || e.route != "/api-keys/{id}/rotate" {
		t.Errorf("api_key.rotated = %+v", e)
	}
	var viaKey *entry
	for i := range got {
		if got[i].action == "POST /tags" && got[i].apiKeyID != "" {
			viaKey = &got[i]
		}
	}
	if viaKey == nil || viaKey.apiKeyID != created.ApiKey.ID || viaKey.actor != env.userA || viaKey.entityType != "tags" || viaKey.entityID == "" {
		t.Errorf("write through the key = %+v, want its key id, creator and the created tag (rows %+v)", viaKey, got)
	}
	for _, e := range got {
		if strings.HasPrefix(e.route, "/analytics/") {
			t.Errorf("a POST-shaped read was audited: %+v", e)
		}
		if strings.Contains(e.action+e.entityID, created.FullKey) || strings.Contains(e.action+e.entityID, rotated.FullKey) {
			t.Fatal("a key secret reached the audit trail")
		}
	}
}

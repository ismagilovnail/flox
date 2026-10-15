package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := seed(db); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(newHandler(db))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, body string, hdr map[string]string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func TestAPIStatusChangeShowsOnDashboard(t *testing.T) {
	srv := newTestServer(t)

	code, b := do(t, srv, "GET", "/api/stats", "", nil)
	if code != 200 {
		t.Fatalf("stats: %d %s", code, b)
	}
	var before Stats
	if err := json.Unmarshal(b, &before); err != nil {
		t.Fatal(err)
	}
	if before.Totals.All == 0 || len(before.Series) != 30 || len(before.Weekly) != 8 || len(before.ByPriority) != 4 {
		t.Fatalf("stats shape: all=%d series=%d weekly=%d prio=%d", before.Totals.All, len(before.Series), len(before.Weekly), len(before.ByPriority))
	}

	if code, b := do(t, srv, "PATCH", "/api/items/DATA-1", `{"status":"done"}`, nil); code != 200 {
		t.Fatalf("patch: %d %s", code, b)
	}
	_, b = do(t, srv, "GET", "/api/stats?days=7", "", nil)
	var after Stats
	if err := json.Unmarshal(b, &after); err != nil {
		t.Fatal(err)
	}
	if after.Totals.Done != before.Totals.Done+1 || after.Totals.P0Open != before.Totals.P0Open-1 || after.Totals.DoneLast7 != before.Totals.DoneLast7+1 {
		t.Errorf("totals before %+v after %+v", before.Totals, after.Totals)
	}
	if len(after.Series) != 7 || after.Series[6].P0Open != after.Totals.P0Open {
		t.Errorf("today's point %+v, totals p0Open %d", after.Series[len(after.Series)-1], after.Totals.P0Open)
	}
	if len(after.Recent) == 0 || after.Recent[0].Code != "DATA-1" || after.Recent[0].New != "done" {
		t.Errorf("recent = %+v", after.Recent)
	}
}

func TestAPIStatsDaysBounds(t *testing.T) {
	srv := newTestServer(t)
	for _, d := range []string{"0", "6", "181", "x"} {
		if code, _ := do(t, srv, "GET", "/api/stats?days="+d, "", nil); code != 422 {
			t.Errorf("days=%s: %d, want 422", d, code)
		}
	}
}

func TestAPIGuards(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name, method, path, body string
		hdr                      map[string]string
		want                     int
	}{
		{"foreign host", "GET", "/api/items", "", map[string]string{"Host": "evil.example"}, 403},
		{"cross-origin write", "PATCH", "/api/items/SEC-1", `{"status":"todo"}`, map[string]string{"Origin": "https://evil.example"}, 403},
		{"bad status", "PATCH", "/api/items/SEC-1", `{"status":"lol"}`, nil, 422},
		{"field not editable", "PATCH", "/api/items/SEC-1", `{"code":"X-1"}`, nil, 422},
		{"unknown item", "PATCH", "/api/items/NOPE-1", `{"status":"done"}`, nil, 404},
		{"duplicate code", "POST", "/api/items", `{"code":"SEC-1","title":"x","priority":"P1","severity":"low","kind":"bug"}`, nil, 422},
		{"bad code", "POST", "/api/items", `{"code":"x y","title":"x","priority":"P1","severity":"low","kind":"bug"}`, nil, 422},
		{"create", "POST", "/api/items", `{"code":"TASK-1","title":"Новая задача","priority":"P1","severity":"low","kind":"bug"}`, nil, 201},
		{"panel page", "GET", "/", "", nil, 200},
		{"panel script", "GET", "/app.js", "", nil, 200},
	}
	for _, c := range cases {
		if code, b := do(t, srv, c.method, c.path, c.body, c.hdr); code != c.want {
			t.Errorf("%s: %d %s, want %d", c.name, code, b, c.want)
		}
	}
	// A form post (text/plain) needs no preflight, so it must be refused.
	req, _ := http.NewRequest("POST", srv.URL+"/api/items", strings.NewReader(`{"title":"x"}`))
	req.Header.Set("Content-Type", "text/plain")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain post: %d, want 415", res.StatusCode)
	}
}

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Through the real handler, against a seeded campaign with sticky flow and
// keepClickId on: the tracker's own cookie keeps the visitor and the click
// id; a forged one — the cookie from tech audit T5 — changes nothing.
func TestStickyCookieThroughTheTracker(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, nil)
	if _, err := pool.Exec(context.Background(),
		`UPDATE campaigns SET sticky_flow = true, sticky_flow_keep_click_id = true WHERE organization_id = $1`, fx.orgID); err != nil {
		t.Fatal(err)
	}
	h, sink, closeEvents := newLandingHandler(t, pool, false)

	visit := func(cookie *http.Cookie) (*httptest.ResponseRecorder, *http.Cookie) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://"+fx.host+"/t/go", nil)
		req.Host = fx.host
		req.RemoteAddr = "198.51.100.7:12345"
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) Chrome/126.0 Mobile")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", rec.Code)
		}
		for _, c := range rec.Result().Cookies() {
			if strings.HasPrefix(c.Name, "sf_") {
				return rec, c
			}
		}
		t.Fatal("no sticky cookie set")
		return nil, nil
	}

	_, first := visit(nil)
	set, flowAndRest, _ := strings.Cut(strings.SplitN(first.Value, ".", 2)[0], ":")
	flow, firstClick, _ := strings.Cut(flowAndRest, ":")
	if !idgen.IsValid(set) || !idgen.IsValid(flow) || !idgen.IsValid(firstClick) || !strings.Contains(first.Value, ".") {
		t.Fatalf("first cookie = %q, want setId:flowId:clickId.signature", first.Value)
	}

	// Returning visitor with the tracker's own cookie.
	visit(first)
	forgedClick := idgen.New()
	// Forged: unsigned, then under the first cookie's signature.
	visit(&http.Cookie{Name: first.Name, Value: set + ":" + flow + ":" + forgedClick})
	visit(&http.Cookie{Name: first.Name, Value: set + ":" + flow + ":" + forgedClick + strings.TrimPrefix(first.Value, set+":"+flow+":"+firstClick)})
	closeEvents()

	var clicks []event.Event
	for _, e := range sink.events {
		if e.Type == event.SourceClick {
			clicks = append(clicks, e)
		}
	}
	if len(clicks) != 4 {
		t.Fatalf("%d clicks recorded, want 4", len(clicks))
	}
	if clicks[0].StickyApplied || clicks[0].ClickID != firstClick {
		t.Errorf("first visit: sticky %v, click %s — want a fresh assignment with the cookie's click", clicks[0].StickyApplied, clicks[0].ClickID)
	}
	if !clicks[1].StickyApplied || clicks[1].ClickID != firstClick {
		t.Errorf("return with the tracker's cookie: sticky %v, click %s — want sticky and the original click %s", clicks[1].StickyApplied, clicks[1].ClickID, firstClick)
	}
	for i, c := range clicks[2:] {
		if c.StickyApplied || c.ClickID == forgedClick || c.ClickID == firstClick {
			t.Errorf("forged cookie %d: sticky %v, click %s — want it ignored and a new click id", i+1, c.StickyApplied, c.ClickID)
		}
	}
}

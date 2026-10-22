package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/visitorkey"
)

// Audit DATA-5: unique clicks count browsers by a visitor cookie. The
// first click sets it; a returning browser keeps its visitor; two phones
// behind one carrier IP with the same user agent are two visitors — which
// IP + user agent counted as one. The old pseudonym is kept beside it.
func TestVisitorCookie(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, nil)
	key := bytes.Repeat([]byte{9}, 32)
	h, sink, closeEvents := newLandingHandler(t, pool, false, func(h *Handler) { h.visitorKey = key })

	click := func(cookie string) (string, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(http.MethodGet, "http://"+fx.host+"/t/"+fx.slug, nil)
		req.Host = fx.host
		req.RemoteAddr = "100.64.0.1:40000" // one carrier NAT address
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Mobile/15E148")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: visitorkey.CookieName, Value: cookie})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Name == visitorkey.CookieName {
				return c.Value, rec
			}
		}
		return "", rec
	}

	first, rec := click("")
	if !visitorkey.ValidID(first) || rec.Code != http.StatusFound {
		t.Fatalf("first click: cookie %q, status %d — want a new visitor cookie", first, rec.Code)
	}
	if again, _ := click(first); again != "" {
		t.Errorf("a returning browser was given a new cookie %q", again)
	}
	second, _ := click("")    // another phone, same IP and user agent
	replaced, _ := click("x") // a malformed cookie is replaced
	if second == "" || second == first || !visitorkey.ValidID(replaced) {
		t.Fatalf("second phone %q, replaced %q", second, replaced)
	}
	closeEvents()

	if len(sink.events) != 4 {
		t.Fatalf("%d events, want 4 clicks", len(sink.events))
	}
	e := sink.events
	if e[0].VisitorKey != visitorkey.FromCookie(key, first) || e[1].VisitorKey != e[0].VisitorKey {
		t.Errorf("returning browser: keys %q, %q — want the same cookie visitor", e[0].VisitorKey, e[1].VisitorKey)
	}
	if e[2].VisitorKey == e[0].VisitorKey {
		t.Error("two phones behind one IP with the same user agent are still one visitor")
	}
	ipKey := visitorkey.Of(key, "100.64.0.1", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Mobile/15E148")
	for i, ev := range e {
		if ev.IPVisitorKey != ipKey {
			t.Errorf("click %d: ip visitor key %q, want the IP + user agent pseudonym", i, ev.IPVisitorKey)
		}
	}
}

// Without VISITOR_KEY there is no cookie and no key: the report falls back
// to IP + user agent, as before.
func TestNoVisitorCookieWithoutKey(t *testing.T) {
	pool := testPool(t)
	fx := seedFlowFixture(t, pool, nil, nil)
	h, sink, closeEvents := newLandingHandler(t, pool, false)
	rec := get(h, fx.host, "/t/"+fx.slug)
	closeEvents()
	for _, c := range rec.Result().Cookies() {
		if c.Name == visitorkey.CookieName {
			t.Errorf("visitor cookie %q set without a key", c.Value)
		}
	}
	if len(sink.events) != 1 || sink.events[0].VisitorKey != "" || sink.events[0].IPVisitorKey != "" {
		t.Errorf("events = %+v, want one click with no keys", sink.events)
	}
}

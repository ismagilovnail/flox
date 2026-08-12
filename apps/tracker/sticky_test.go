package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

const testCampaignID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func requestWithCookie(value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/t/abc", nil)
	if value != "" {
		r.AddCookie(&http.Cookie{Name: stickyCookieName(testCampaignID), Value: value})
	}
	return r
}

func TestParseStickyCookie(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  *stickyCookie
	}{
		{"no cookie", "", nil},
		{"set and flow", "set1:flow1", &stickyCookie{StreamSetID: "set1", FlowID: "flow1"}},
		{"set, flow and click id", "set1:flow1:click1", &stickyCookie{StreamSetID: "set1", FlowID: "flow1", ClickID: "click1"}},
		{"malformed — only one part", "set1", nil},
		{"malformed — empty set id", ":flow1", nil},
		{"malformed — empty flow id", "set1:", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseStickyCookie(requestWithCookie(tc.value), testCampaignID)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("got %+v, want nil (malformed cookies must be ignored, not half-parsed)", got)
			case tc.want != nil && got == nil:
				t.Fatalf("got nil, want %+v", tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestStickyCookieRoutingStateOmitsClickID(t *testing.T) {
	// The routing engine must only ever receive the two fields a routing
	// decision depends on — the click_id is attribution-only.
	sc := &stickyCookie{StreamSetID: "set1", FlowID: "flow1", ClickID: "click1"}
	state := sc.routingState()
	if state.StreamSetID != "set1" || state.FlowID != "flow1" {
		t.Fatalf("routingState() = %+v", state)
	}

	var nilCookie *stickyCookie
	if nilCookie.routingState() != nil {
		t.Fatal("a nil cookie must produce a nil StickyState, not an empty one that looks like a real assignment")
	}
}

func TestSetStickyCookie(t *testing.T) {
	result := routing.RouteResult{StreamSetID: "set1", FlowID: "flow1"}

	t.Run("without keepClickId", func(t *testing.T) {
		w := httptest.NewRecorder()
		setStickyCookie(w, testCampaignID, result, "click1", false)

		c := parseSetCookie(t, w)
		if c.Value != "set1:flow1" {
			t.Fatalf("value = %q, want %q", c.Value, "set1:flow1")
		}
	})

	t.Run("with keepClickId", func(t *testing.T) {
		w := httptest.NewRecorder()
		setStickyCookie(w, testCampaignID, result, "click1", true)

		c := parseSetCookie(t, w)
		if c.Value != "set1:flow1:click1" {
			t.Fatalf("value = %q, want %q", c.Value, "set1:flow1:click1")
		}
	})

	t.Run("is long-lived so it survives restarts and cross-session returns", func(t *testing.T) {
		w := httptest.NewRecorder()
		setStickyCookie(w, testCampaignID, result, "click1", false)

		c := parseSetCookie(t, w)
		if c.MaxAge < 30*24*60*60 {
			t.Fatalf("MaxAge = %d — too short; a short-lived sticky cookie silently re-buckets returning users and corrupts A/B tests", c.MaxAge)
		}
		if !c.HttpOnly {
			t.Error("sticky cookie should be HttpOnly")
		}
	})
}

func TestClickIDFor(t *testing.T) {
	sticky := &stickyCookie{StreamSetID: "set1", FlowID: "flow1", ClickID: "original-click"}
	applied := routing.RouteResult{StickyApplied: true}
	fresh := routing.RouteResult{StickyApplied: false}

	t.Run("reuses the original click id when sticky applied and keepClickId is on", func(t *testing.T) {
		if got := clickIDFor(sticky, applied, true); got != "original-click" {
			t.Fatalf("got %q, want the original click id preserved for attribution", got)
		}
	})

	t.Run("mints a new id when keepClickId is off", func(t *testing.T) {
		if got := clickIDFor(sticky, applied, false); got == "original-click" || got == "" {
			t.Fatalf("got %q, want a freshly minted click id", got)
		}
	})

	t.Run("mints a new id when sticky was not applied", func(t *testing.T) {
		if got := clickIDFor(sticky, fresh, true); got == "original-click" || got == "" {
			t.Fatalf("got %q, want a freshly minted click id", got)
		}
	})

	t.Run("mints a new id when there is no cookie", func(t *testing.T) {
		if got := clickIDFor(nil, applied, true); got == "" {
			t.Fatal("want a freshly minted click id")
		}
	})
}

func parseSetCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	res := w.Result()
	defer res.Body.Close()
	cookies := res.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	return cookies[0]
}

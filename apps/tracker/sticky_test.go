package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/metrics"
	"github.com/ismagilovnail/flox/apps/internal/routing"
)

const (
	testCampaignID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	testSetID      = "01M4DFKTGA45HC5RRJQ9X4RH7S"
	testFlowID     = "01M4DFKTGDHH3A50G4T6NZZVVV"
	testClickID    = "01M4FJYE49DJ0HX9D0YGBDSNVK"
)

func testKey(b byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(rune('a'+b)), 32)))
}

func testStickySigner(tb testing.TB) *stickySigner {
	tb.Helper()
	s, err := newStickySigner(testKey(0))
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func requestWithCookie(campaignID, value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/t/abc", nil)
	if value != "" {
		r.AddCookie(&http.Cookie{Name: stickyCookieName(campaignID), Value: value})
	}
	return r
}

// issued is the cookie value the tracker sets for this result.
func issued(t *testing.T, s *stickySigner, campaignID string, keepClickID bool) string {
	t.Helper()
	w := httptest.NewRecorder()
	s.set(w, campaignID, routing.RouteResult{StreamSetID: testSetID, FlowID: testFlowID}, testClickID, keepClickID)
	return parseSetCookie(t, w).Value
}

func TestStickyCookieRoundTrip(t *testing.T) {
	s := testStickySigner(t)
	for _, keep := range []bool{false, true} {
		value := issued(t, s, testCampaignID, keep)
		got := s.parse(requestWithCookie(testCampaignID, value), testCampaignID)
		want := stickyCookie{StreamSetID: testSetID, FlowID: testFlowID}
		if keep {
			want.ClickID = testClickID
		}
		if got == nil || *got != want {
			t.Errorf("keepClickId=%v: parse(%q) = %+v, want %+v", keep, value, got, want)
		}
	}
}

// A cookie the tracker did not issue is ignored: forging one used to skip
// the stream set's filters straight into its flow (tech audit T5).
func TestStickyCookieNotIssuedHereIsIgnored(t *testing.T) {
	s := testStickySigner(t)
	other, _ := newStickySigner(testKey(1))
	good := issued(t, s, testCampaignID, true)
	value, sig, _ := strings.Cut(good, ".")
	otherCampaign := idgen.New()

	for _, c := range []struct {
		name, campaignID, cookie, reason string
	}{
		{"unsigned, the old format", testCampaignID, testSetID + ":" + testFlowID, stickyUnsigned},
		{"unsigned with a click id", testCampaignID, value, stickyUnsigned},
		{"another flow under the same signature", testCampaignID, testSetID + ":" + idgen.New() + ":" + testClickID + "." + sig, stickyBadSignature},
		{"another click id under the same signature", testCampaignID, testSetID + ":" + testFlowID + ":" + idgen.New() + "." + sig, stickyBadSignature},
		{"another campaign's cookie", otherCampaign, good, stickyBadSignature},
		{"another key", testCampaignID, issued(t, other, testCampaignID, true), stickyBadSignature},
		{"truncated signature", testCampaignID, good[:len(good)-1], stickyBadSignature},
		{"junk", testCampaignID, strings.Repeat("x", 4000) + "." + strings.Repeat("y", 22), stickyBadSignature},
		{"signed, but the click id is not one", testCampaignID, signed(s, testCampaignID, testSetID+":"+testFlowID+":"+strings.Repeat("z", 3000)), stickyMalformed},
		{"signed, one part", testCampaignID, signed(s, testCampaignID, testSetID), stickyMalformed},
		{"signed, four parts", testCampaignID, signed(s, testCampaignID, testSetID+":"+testFlowID+":"+testClickID+":"+testClickID), stickyMalformed},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := testutil.ToFloat64(metrics.StickyCookiesRejectedTotal.WithLabelValues(c.reason))
			if got := s.parse(requestWithCookie(c.campaignID, c.cookie), c.campaignID); got != nil {
				t.Fatalf("parse = %+v, want the cookie ignored", got)
			}
			if after := testutil.ToFloat64(metrics.StickyCookiesRejectedTotal.WithLabelValues(c.reason)); after != before+1 {
				t.Errorf("rejected{reason=%q} went %v → %v, want +1", c.reason, before, after)
			}
		})
	}
}

func signed(s *stickySigner, campaignID, value string) string {
	return value + "." + s.sign(campaignID, value)
}

func TestNoStickyCookie(t *testing.T) {
	if got := testStickySigner(t).parse(requestWithCookie(testCampaignID, ""), testCampaignID); got != nil {
		t.Fatalf("parse = %+v, want nil", got)
	}
}

func TestNewStickySignerRefusesABadKey(t *testing.T) {
	for name, key := range map[string]string{
		"empty":      "",
		"not base64": "not base64!",
		"short":      base64.StdEncoding.EncodeToString([]byte("16 bytes, short.")),
	} {
		if _, err := newStickySigner(key); err == nil {
			t.Errorf("%s key accepted", name)
		}
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

func TestSetStickyCookieAttributes(t *testing.T) {
	w := httptest.NewRecorder()
	testStickySigner(t).set(w, testCampaignID, routing.RouteResult{StreamSetID: testSetID, FlowID: testFlowID}, testClickID, false)
	c := parseSetCookie(t, w)
	if c.MaxAge < 30*24*60*60 {
		t.Fatalf("MaxAge = %d — too short; a short-lived sticky cookie silently re-buckets returning users and corrupts A/B tests", c.MaxAge)
	}
	if !c.HttpOnly || !c.Secure {
		t.Error("sticky cookie should be HttpOnly and Secure")
	}
	if !strings.HasPrefix(c.Value, testSetID+":"+testFlowID+".") {
		t.Errorf("value = %q, want setId:flowId.signature", c.Value)
	}
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

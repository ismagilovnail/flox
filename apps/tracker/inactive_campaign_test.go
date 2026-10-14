package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// §58 "inactive campaigns": internal/routing carries no campaign status
// (TestInactiveCampaigns_CallerLevelConcern), so the tracker is where it is
// enforced. A paused, draft or archived campaign's tracking link answers
// 404 and records nothing — no click, no filtered click — so its traffic
// neither reaches an offer nor shows up as clicks in its reports. The active
// campaign is the control: the same fixture redirects and records a click.
func TestInactiveCampaignIsNotTracked(t *testing.T) {
	pool := testPool(t)
	for _, status := range []string{"active", "paused", "draft", "archived"} {
		t.Run(status, func(t *testing.T) {
			fx := seedFlowFixture(t, pool, nil, nil)
			if _, err := pool.Exec(context.Background(),
				`UPDATE campaigns SET status = $2 WHERE organization_id = $1`, fx.orgID, status); err != nil {
				t.Fatal(err)
			}
			h, sink, closeEvents := newLandingHandler(t, pool, false)

			req := httptest.NewRequest(http.MethodGet, "http://"+fx.host+"/t/"+fx.slug+"?sub1=ad-1", nil)
			req.Host = fx.host
			req.RemoteAddr = "198.51.100.9:12345"
			req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) Chrome/126.0 Mobile")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			closeEvents()

			if status == "active" {
				if rec.Code != http.StatusFound || rec.Header().Get("Location") != landingTestOffer {
					t.Fatalf("active campaign: %d → %q, want 302 to the flow's destination", rec.Code, rec.Header().Get("Location"))
				}
				if len(sink.events) != 1 {
					t.Fatalf("active campaign: %d events, want the click", len(sink.events))
				}
				return
			}
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s campaign: status %d, want 404", status, rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Fatalf("%s campaign redirected to %q (the fallback or an offer), want no redirect", status, loc)
			}
			if len(sink.events) != 0 {
				t.Fatalf("%s campaign recorded %d events (%v), want none", status, len(sink.events), sink.events[0].Type)
			}
		})
	}
}

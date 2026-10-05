package routing_test

import (
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

// TestRouteResultOfferID: OfferID names the offer the visitor was actually
// sent to — and nothing else. It feeds the `offer` report dimension, so a
// fallback after an inactive offer, or a redirect flow, must leave it empty.
func TestRouteResultOfferID(t *testing.T) {
	withOffer := func(active bool) routing.Destination {
		d := offerTo("https://offer.example", active)
		d.OfferID = "offer-1"
		return d
	}
	cases := []struct {
		name      string
		dest      routing.Destination
		wantDest  string
		wantOffer string
	}{
		{"active offer", withOffer(true), "https://offer.example", "offer-1"},
		{"inactive offer falls back", withOffer(false), "https://set-fallback.example", ""},
		{"redirect flow", redirectTo("https://redirect.example"), "https://redirect.example", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := routing.RoutingConfig{
				CampaignID: "c1", FallbackURL: "https://fallback.example", StickyFlow: true,
				StreamSets: []routing.StreamSet{{
					ID: "set1", Priority: 1, Status: routing.StreamSetActive, FallbackURL: "https://set-fallback.example",
					Flows: []routing.Flow{flow("f1", true, 100, tc.dest)},
				}},
			}
			e := &routing.Engine{}

			fresh, err := e.Resolve(ctx(), routing.RequestContext{Config: cfg, VisitKey: "v"})
			if err != nil || fresh.Destination != tc.wantDest || fresh.OfferID != tc.wantOffer {
				t.Fatalf("fresh: got dest %q offer %q (err %v), want %q / %q", fresh.Destination, fresh.OfferID, err, tc.wantDest, tc.wantOffer)
			}

			sticky, err := e.Resolve(ctx(), routing.RequestContext{
				Config: cfg, Sticky: &routing.StickyState{StreamSetID: "set1", FlowID: "f1"},
			})
			if err != nil || !sticky.StickyApplied || sticky.OfferID != tc.wantOffer {
				t.Fatalf("sticky: got %+v (err %v), want offer %q", sticky, err, tc.wantOffer)
			}
		})
	}
}

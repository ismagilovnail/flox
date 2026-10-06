package routing_test

import (
	"strings"
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

func cappedOfferFlow(id string, weight int, offerID, url string) routing.Flow {
	return flow(id, true, weight, routing.Destination{
		Kind: routing.DestinationOffer, URL: url, OfferActive: true, OfferID: offerID,
	})
}

func capConfig(sticky bool, flows ...routing.Flow) routing.RoutingConfig {
	return routing.RoutingConfig{
		CampaignID:  "c1",
		FallbackURL: "https://fallback.example",
		StickyFlow:  sticky,
		StreamSets: []routing.StreamSet{{
			ID: "set1", Name: "set1", Priority: 1, Status: routing.StreamSetActive,
			RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
			Flows:      flows,
		}},
	}
}

func TestCappedOfferLeavesTheDraw(t *testing.T) {
	cfg := capConfig(false,
		cappedOfferFlow("fa", 50, "offer-a", "https://a.example"),
		cappedOfferFlow("fb", 50, "offer-b", "https://b.example"),
	)
	e := &routing.Engine{}
	capped := map[string]struct{}{"offer-a": {}}

	// Every visit lands on B, whatever its hash would have said without a cap.
	for _, key := range []string{"k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8"} {
		res, err := e.Resolve(ctx(), routing.RequestContext{Config: cfg, VisitKey: key, CappedOffers: capped})
		if err != nil {
			t.Fatalf("key %s: %v", key, err)
		}
		if res.FlowID != "fb" || res.OfferID != "offer-b" {
			t.Fatalf("key %s: got flow %q offer %q, want fb/offer-b", key, res.FlowID, res.OfferID)
		}
	}
}

func TestCappedOfferExplainMarksCandidate(t *testing.T) {
	cfg := capConfig(false,
		cappedOfferFlow("fa", 50, "offer-a", "https://a.example"),
		cappedOfferFlow("fb", 50, "offer-b", "https://b.example"),
	)
	_, expl, err := (&routing.Engine{}).Explain(ctx(), routing.RequestContext{
		Config: cfg, VisitKey: "k", CappedOffers: map[string]struct{}{"offer-a": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !expl.FlowCandidates[0].Capped || expl.FlowCandidates[0].NormalizedPercent != 0 {
		t.Fatalf("flow A candidate = %+v, want capped with 0%%", expl.FlowCandidates[0])
	}
	if expl.FlowCandidates[1].Capped || expl.FlowCandidates[1].NormalizedPercent != 100 {
		t.Fatalf("flow B candidate = %+v, want uncapped with 100%%", expl.FlowCandidates[1])
	}
}

func TestAllOffersCappedFallsBack(t *testing.T) {
	cfg := capConfig(false, cappedOfferFlow("fa", 100, "offer-a", "https://a.example"))
	res, err := (&routing.Engine{}).Resolve(ctx(), routing.RequestContext{
		Config: cfg, CappedOffers: map[string]struct{}{"offer-a": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Destination != "https://fallback.example" || res.OfferID != "" {
		t.Fatalf("got %+v, want the campaign fallback and no offer", res)
	}
	if !strings.Contains(res.Reason, "daily cap") {
		t.Fatalf("reason %q should mention the daily cap", res.Reason)
	}
}

func TestCapDoesNotTouchRedirectFlowsOrOtherOffers(t *testing.T) {
	cfg := capConfig(false,
		flow("fr", true, 100, redirectTo("https://r.example")),
	)
	res, _ := (&routing.Engine{}).Resolve(ctx(), routing.RequestContext{
		Config: cfg, CappedOffers: map[string]struct{}{"offer-a": {}},
	})
	if res.FlowID != "fr" {
		t.Fatalf("redirect flow must stay eligible, got %+v", res)
	}
	if len(cfg.StreamSets[0].Flows) != 1 || !cfg.StreamSets[0].Flows[0].Active {
		t.Fatal("caps must not modify the caller's config")
	}
}

func TestStickyToCappedOfferIsReEvaluated(t *testing.T) {
	cfg := capConfig(true,
		cappedOfferFlow("fa", 50, "offer-a", "https://a.example"),
		cappedOfferFlow("fb", 50, "offer-b", "https://b.example"),
	)
	e := &routing.Engine{}
	sticky := &routing.StickyState{StreamSetID: "set1", FlowID: "fa"}

	res, err := e.Resolve(ctx(), routing.RequestContext{Config: cfg, Sticky: sticky, VisitKey: "k"})
	if err != nil || !res.StickyApplied || res.FlowID != "fa" {
		t.Fatalf("without a cap sticky is honored, got %+v err %v", res, err)
	}

	res, err = e.Resolve(ctx(), routing.RequestContext{
		Config: cfg, Sticky: sticky, VisitKey: "k", CappedOffers: map[string]struct{}{"offer-a": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.StickyApplied || res.FlowID != "fb" {
		t.Fatalf("capped sticky offer must be re-evaluated onto fb, got %+v", res)
	}
}

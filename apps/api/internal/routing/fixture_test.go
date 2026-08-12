package routing_test

import (
	"context"
	"math"
	"testing"

	"github.com/ismagilovnail/flox/apps/api/internal/routing"
)

// fixture_test.go is the shared conformance fixture §6-SHARED requires:
// every case §58 lists, run against the real Go engine. There is no
// parallel TypeScript implementation to also run these against — Strategy
// A (ARCHITECTURE.md) means the frontend Routing Simulator is a thin UI
// over this exact engine's future HTTP wrapper (Phase 27), never a second
// implementation of the decision logic itself.
//
// Three of §58's cases are deliberately not exercised here, because they
// aren't this package's decision to make — routing.Engine only ever runs
// after the caller (apps/tracker, Phase 21) has already resolved a valid
// tracking link, confirmed the campaign itself is active, and handled any
// in-app WebView bounce:
//   - "invalid tracking links" — the caller never calls Resolve at all if
//     the tracking link doesn't resolve to a campaign.
//   - "inactive campaigns" — RoutingConfig deliberately carries no
//     campaign-level status; a caller loading config for a paused/
//     archived/draft campaign routes straight to a safe destination
//     without invoking this engine.
//   - "in-app WebView bounce" — a pre-routing HTTP-layer redirect, not a
//     stream-set/flow decision.

func ctx() context.Context { return context.Background() }

func flow(id string, active bool, weight int, dest routing.Destination) routing.Flow {
	return routing.Flow{ID: id, Name: id, Active: active, Weight: weight, Destination: dest}
}

func redirectTo(url string) routing.Destination {
	return routing.Destination{Kind: routing.DestinationRedirect, URL: url}
}

func offerTo(url string, active bool) routing.Destination {
	return routing.Destination{Kind: routing.DestinationOffer, URL: url, OfferActive: active}
}

func cond(field routing.FilterField, op routing.FilterOperator, value string) routing.FilterCondition {
	return routing.FilterCondition{Field: field, Operator: op, Value: value}
}

func TestFilterEvaluation_AND(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND, Children: []routing.FilterNode{
			cond(routing.FieldCountry, routing.OpIs, "US"),
			cond(routing.FieldDevice, routing.OpIs, "mobile"),
		}},
		Flows: []routing.Flow{flow("f1", true, 100, redirectTo("https://match.example"))},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", FallbackURL: "https://fallback.example", StreamSets: []routing.StreamSet{set}}
	e := &routing.Engine{}

	t.Run("both conditions true -> matches", func(t *testing.T) {
		res, err := e.Resolve(ctx(), routing.RequestContext{
			Attributes: routing.Attributes{routing.FieldCountry: "US", routing.FieldDevice: "mobile"}, Config: cfg,
		})
		if err != nil || res.Destination != "https://match.example" {
			t.Fatalf("got %+v, err %v", res, err)
		}
	})

	t.Run("one condition false -> falls through to fallback", func(t *testing.T) {
		res, _ := e.Resolve(ctx(), routing.RequestContext{
			Attributes: routing.Attributes{routing.FieldCountry: "US", routing.FieldDevice: "desktop"}, Config: cfg,
		})
		if res.Destination != "https://fallback.example" {
			t.Fatalf("want fallback, got %+v", res)
		}
	})
}

func TestFilterEvaluation_OR(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinOR, Children: []routing.FilterNode{
			cond(routing.FieldCountry, routing.OpIs, "US"),
			cond(routing.FieldCountry, routing.OpIs, "CA"),
		}},
		Flows: []routing.Flow{flow("f1", true, 100, redirectTo("https://match.example"))},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}
	e := &routing.Engine{}

	for _, country := range []string{"US", "CA"} {
		res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{routing.FieldCountry: country}, Config: cfg})
		if res.Destination != "https://match.example" {
			t.Fatalf("country %s: want match, got %+v", country, res)
		}
	}

	res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{routing.FieldCountry: "DE"}, Config: cfg})
	if res.Destination != "" {
		t.Fatalf("country DE: want no destination, got %+v", res)
	}
}

func TestFilterEvaluation_NestedGroups(t *testing.T) {
	// OR( country IS US, AND( device IS mobile, bot IS 0 ) )
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinOR, Children: []routing.FilterNode{
			cond(routing.FieldCountry, routing.OpIs, "US"),
			routing.FilterGroup{Joiner: routing.JoinAND, Children: []routing.FilterNode{
				cond(routing.FieldDevice, routing.OpIs, "mobile"),
				cond(routing.FieldBot, routing.OpIs, "0"),
			}},
		}},
		Flows: []routing.Flow{flow("f1", true, 100, redirectTo("https://match.example"))},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}
	e := &routing.Engine{}

	// Matches via the nested AND branch even though country fails.
	res, _ := e.Resolve(ctx(), routing.RequestContext{
		Attributes: routing.Attributes{routing.FieldCountry: "DE", routing.FieldDevice: "mobile", routing.FieldBot: "0"}, Config: cfg,
	})
	if res.Destination != "https://match.example" {
		t.Fatalf("want nested-AND match, got %+v", res)
	}

	// Neither branch satisfied.
	res, _ = e.Resolve(ctx(), routing.RequestContext{
		Attributes: routing.Attributes{routing.FieldCountry: "DE", routing.FieldDevice: "desktop", routing.FieldBot: "0"}, Config: cfg,
	})
	if res.Destination != "" {
		t.Fatalf("want no match, got %+v", res)
	}
}

func TestPriority_FirstMatchWins(t *testing.T) {
	// Two sets both match; priority 1 must win even though it's declared
	// second in the slice — proves sorting, not declaration order, governs.
	low := routing.StreamSet{
		ID: "low-priority-declared-first", Priority: 2, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
		Flows:      []routing.Flow{flow("f-low", true, 100, redirectTo("https://low.example"))},
	}
	high := routing.StreamSet{
		ID: "high-priority-declared-second", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
		Flows:      []routing.Flow{flow("f-high", true, 100, redirectTo("https://high.example"))},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{low, high}}
	e := &routing.Engine{}

	res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
	if res.StreamSetID != "high-priority-declared-second" || res.Destination != "https://high.example" {
		t.Fatalf("want priority-1 set to win, got %+v", res)
	}
}

func TestFallback_CampaignLevel(t *testing.T) {
	cfg := routing.RoutingConfig{CampaignID: "c1", FallbackURL: "https://campaign-fallback.example", StreamSets: nil}
	e := &routing.Engine{}
	res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
	if res.Destination != "https://campaign-fallback.example" {
		t.Fatalf("want campaign fallback, got %+v", res)
	}
}

func TestFallback_StreamSetLevelBeforeCampaign(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter:  routing.FilterGroup{Joiner: routing.JoinAND},
		Flows:       nil, // matches, but has no flow at all
		FallbackURL: "https://streamset-fallback.example",
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", FallbackURL: "https://campaign-fallback.example", StreamSets: []routing.StreamSet{set}}
	e := &routing.Engine{}
	res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
	if res.Destination != "https://streamset-fallback.example" {
		t.Fatalf("want stream-set fallback (not campaign fallback), got %+v", res)
	}
}

func TestWeightedRouting_DistributionWithin2PercentOver10kPicks(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
		Flows: []routing.Flow{
			flow("f70", true, 70, redirectTo("https://a.example")),
			flow("f30", true, 30, redirectTo("https://b.example")),
		},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}

	// A real PRNG, not a fixed value — this test needs actual statistical
	// spread across trials, which is why Engine.Rand01 is injectable.
	var seed uint64 = 42
	e := &routing.Engine{Rand01: pseudoRandSource(&seed)}

	const trials = 10_000
	counts := map[string]int{}
	for i := 0; i < trials; i++ {
		res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
		counts[res.FlowID]++
	}

	pctA := float64(counts["f70"]) / trials * 100
	pctB := float64(counts["f30"]) / trials * 100
	if math.Abs(pctA-70) > 2 {
		t.Fatalf("f70 got %.2f%%, want 70%% ± 2%%", pctA)
	}
	if math.Abs(pctB-30) > 2 {
		t.Fatalf("f30 got %.2f%%, want 30%% ± 2%%", pctB)
	}
}

func TestSticky_SurvivesAcrossCallsWithNoHiddenState(t *testing.T) {
	// This package has no Redis/cache dependency at all — a sticky decision
	// is a pure function of (req.Sticky, req.Config) every single call, so
	// there is nothing that "eviction" could invalidate on this side. This
	// test proves that: calling Resolve N times with the identical sticky
	// cookie value always honors the identical assignment, exactly as it
	// would after a hypothetical Redis flush, because Redis was never
	// consulted in the first place (§39-STICKY: "cookie is truth, Redis is
	// cache only").
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
		Flows: []routing.Flow{
			flow("sticky-flow", true, 1, redirectTo("https://sticky.example")),
			flow("other-flow", true, 99, redirectTo("https://other.example")),
		},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StickyFlow: true, StreamSets: []routing.StreamSet{set}}
	sticky := &routing.StickyState{StreamSetID: "set1", FlowID: "sticky-flow"}
	e := &routing.Engine{}

	for i := 0; i < 5; i++ {
		res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg, Sticky: sticky})
		if !res.StickyApplied || res.FlowID != "sticky-flow" || res.Destination != "https://sticky.example" {
			t.Fatalf("call %d: sticky assignment not honored, got %+v", i, res)
		}
	}
}

func TestSticky_KeepClickId_NotThisPackagesConcern(t *testing.T) {
	t.Skip("stickyFlowKeepClickId only affects whether the caller (apps/tracker) reuses an old click_id for attribution — it has zero effect on which flow gets selected, so RoutingConfig doesn't carry it at all (see types.go doc comment). Nothing for this package to test.")
}

func TestSticky_SkipInactive(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
		Flows: []routing.Flow{
			flow("inactive-flow", false, 1, redirectTo("https://sticky.example")),
			flow("other-flow", true, 100, redirectTo("https://fresh-pick.example")),
		},
	}
	sticky := &routing.StickyState{StreamSetID: "set1", FlowID: "inactive-flow"}
	e := &routing.Engine{}

	t.Run("skipInactive=true keeps the inactive flow anyway", func(t *testing.T) {
		cfg := routing.RoutingConfig{CampaignID: "c1", StickyFlow: true, StickyFlowSkipInactive: true, StreamSets: []routing.StreamSet{set}}
		res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg, Sticky: sticky})
		if !res.StickyApplied || res.FlowID != "inactive-flow" {
			t.Fatalf("want inactive sticky flow kept, got %+v", res)
		}
	})

	t.Run("skipInactive=false drops the cookie and re-picks", func(t *testing.T) {
		cfg := routing.RoutingConfig{CampaignID: "c1", StickyFlow: true, StickyFlowSkipInactive: false, StreamSets: []routing.StreamSet{set}}
		res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg, Sticky: sticky})
		if res.StickyApplied || res.FlowID != "other-flow" {
			t.Fatalf("want fresh pick of the only active flow, got %+v", res)
		}
	})
}

func TestInactiveFlow_ExcludedFromSelection(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
		Flows: []routing.Flow{
			flow("inactive", false, 1000, redirectTo("https://never.example")),
			flow("active", true, 1, redirectTo("https://always.example")),
		},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}
	e := &routing.Engine{}
	res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
	if res.FlowID != "active" {
		t.Fatalf("want only the active flow ever selected, got %+v", res)
	}
}

func TestInactiveOffer_TreatedAsMissingDestination(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter:  routing.FilterGroup{Joiner: routing.JoinAND},
		Flows:       []routing.Flow{flow("f1", true, 100, offerTo("https://inactive-offer.example", false))},
		FallbackURL: "https://streamset-fallback.example",
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}
	e := &routing.Engine{}
	res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
	if res.Destination != "https://streamset-fallback.example" {
		t.Fatalf("want fallback (inactive offer must not be used), got %+v", res)
	}
}

func TestMissingDestination_CascadesThroughFallbacks(t *testing.T) {
	e := &routing.Engine{}

	t.Run("empty redirect URL falls through to stream set fallback", func(t *testing.T) {
		set := routing.StreamSet{
			ID: "set1", Priority: 1, Status: routing.StreamSetActive,
			RootFilter:  routing.FilterGroup{Joiner: routing.JoinAND},
			Flows:       []routing.Flow{flow("f1", true, 100, redirectTo(""))},
			FallbackURL: "https://streamset-fallback.example",
		}
		cfg := routing.RoutingConfig{CampaignID: "c1", FallbackURL: "https://campaign-fallback.example", StreamSets: []routing.StreamSet{set}}
		res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
		if res.Destination != "https://streamset-fallback.example" {
			t.Fatalf("got %+v", res)
		}
	})

	t.Run("no fallback anywhere -> empty destination, not an error", func(t *testing.T) {
		set := routing.StreamSet{
			ID: "set1", Priority: 1, Status: routing.StreamSetActive,
			RootFilter: routing.FilterGroup{Joiner: routing.JoinAND},
			Flows:      []routing.Flow{flow("f1", true, 100, redirectTo(""))},
		}
		cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}
		res, err := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{}, Config: cfg})
		if err != nil {
			t.Fatalf("want no error even with no destination configured, got %v", err)
		}
		if res.Destination != "" {
			t.Fatalf("got %+v", res)
		}
	})
}

func TestISOCodeMismatch_NoFuzzyCoercion(t *testing.T) {
	set := routing.StreamSet{
		ID: "set1", Priority: 1, Status: routing.StreamSetActive,
		RootFilter: routing.FilterGroup{Joiner: routing.JoinAND, Children: []routing.FilterNode{
			cond(routing.FieldCountry, routing.OpIs, "GB"),
		}},
		Flows: []routing.Flow{flow("f1", true, 100, redirectTo("https://match.example"))},
	}
	cfg := routing.RoutingConfig{CampaignID: "c1", StreamSets: []routing.StreamSet{set}}
	e := &routing.Engine{}

	// A request classified as "UK" (not a real ISO 3166-1 alpha-2 code)
	// must NOT match a filter authored against "GB" — this package does
	// no semantic country-code aliasing, only literal (case-insensitive)
	// comparison. If classification ever emits "UK", that's a
	// classifier bug this engine is supposed to surface, not paper over.
	res, _ := e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{routing.FieldCountry: "UK"}, Config: cfg})
	if res.Destination != "" {
		t.Fatalf("want no match for UK against a GB filter, got %+v", res)
	}

	// Case-insensitivity still applies, matching lib/filters.ts's norm().
	res, _ = e.Resolve(ctx(), routing.RequestContext{Attributes: routing.Attributes{routing.FieldCountry: "gb"}, Config: cfg})
	if res.Destination != "https://match.example" {
		t.Fatalf("want case-insensitive match for gb against GB filter, got %+v", res)
	}
}

func TestInvalidTrackingLinks_CallerLevelConcern(t *testing.T) {
	t.Skip("invalid tracking links never reach this package: apps/tracker resolves the tracking link to a campaign before Resolve is ever invoked, and returns a safe response itself when resolution fails.")
}

func TestInactiveCampaigns_CallerLevelConcern(t *testing.T) {
	t.Skip("RoutingConfig deliberately carries no campaign-level status field. The caller only loads/passes RoutingConfig for campaigns it has already confirmed are active; a paused/draft/archived campaign routes to a safe destination without invoking this engine at all.")
}

func TestInAppWebViewBounce_CallerLevelConcern(t *testing.T) {
	t.Skip("the WebView bounce (§73) is a pre-routing HTTP redirect based on User-Agent, handled entirely by apps/tracker before any stream-set/flow decision is made. It never reaches this package.")
}

// pseudoRandSource returns a fast, deterministic (seeded) [0,1) generator
// — good enough for a statistical-distribution test, not cryptographic,
// and doesn't require importing math/rand/v2's *Rand type into the test's
// public surface.
func pseudoRandSource(state *uint64) func() float64 {
	return func() float64 {
		*state ^= *state << 13
		*state ^= *state >> 7
		*state ^= *state << 17
		return float64(*state%1_000_000) / 1_000_000
	}
}

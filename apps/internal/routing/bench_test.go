package routing_test

import (
	"testing"

	"github.com/ismagilovnail/flox/apps/internal/routing"
)

// benchConfig models a realistically-shaped campaign for Phase 31's
// performance benchmark (docs/performance.md): 5 stream sets in priority
// order, each with a 3-condition nested AND/OR filter tree and 3 weighted
// flows — comparable to what apps/tracker's Handler.track actually
// evaluates per click on the §41 hot path, not the minimal single-condition
// fixtures fixture_test.go uses to pin one behavior at a time.
func benchConfig() routing.RoutingConfig {
	sets := make([]routing.StreamSet, 0, 5)
	for i := 0; i < 5; i++ {
		sets = append(sets, routing.StreamSet{
			ID:       "set" + string(rune('1'+i)),
			Priority: i + 1,
			Status:   routing.StreamSetActive,
			RootFilter: routing.FilterGroup{
				Joiner: routing.JoinAND,
				Children: []routing.FilterNode{
					cond(routing.FieldCountry, routing.OpIn, "US,CA,GB,DE,FR"),
					routing.FilterGroup{
						Joiner: routing.JoinOR,
						Children: []routing.FilterNode{
							cond(routing.FieldDevice, routing.OpIs, "mobile"),
							cond(routing.FieldOS, routing.OpIs, "android"),
						},
					},
					cond(routing.FieldBot, routing.OpIs, "0"),
				},
			},
			Flows: []routing.Flow{
				flow("f"+string(rune('1'+i))+"-1", true, 50, redirectTo("https://a.example")),
				flow("f"+string(rune('1'+i))+"-2", true, 30, offerTo("https://b.example", true)),
				flow("f"+string(rune('1'+i))+"-3", true, 20, redirectTo("https://c.example")),
			},
		})
	}
	// Last set is a catch-all so most benchmark draws actually reach the
	// weighted flow selection rather than always falling through to the
	// campaign fallback — the CPU cost §41 actually cares about.
	sets[4].RootFilter = routing.FilterGroup{Joiner: routing.JoinOR, Children: []routing.FilterNode{
		cond(routing.FieldCountry, routing.OpExists, ""),
	}}
	return routing.RoutingConfig{CampaignID: "bench-campaign", FallbackURL: "https://fallback.example", StreamSets: sets}
}

// BenchmarkResolve measures the pure in-memory decision cost internal/
// routing.Engine.Resolve adds to every click — no I/O, no DB, no Redis;
// apps/tracker's own load precedes and follows this call (link/config load,
// classification, async event enqueue) but this is the one piece the
// routing engine itself owns.
func BenchmarkResolve(b *testing.B) {
	e := &routing.Engine{}
	cfg := benchConfig()
	attrs := routing.Attributes{
		routing.FieldCountry: "US",
		routing.FieldDevice:  "mobile",
		routing.FieldOS:      "ios",
		routing.FieldBot:     "0",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.Resolve(ctx(), routing.RequestContext{
			Attributes: attrs,
			Config:     cfg,
			VisitKey:   "bench|1.2.3.4|Mozilla/5.0",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkResolve_Sticky measures the sticky-assignment path (§39-STICKY):
// no filter evaluation, no weighted draw — just honoring the cookie.
func BenchmarkResolve_Sticky(b *testing.B) {
	e := &routing.Engine{}
	cfg := benchConfig()
	cfg.StickyFlow = true
	attrs := routing.Attributes{routing.FieldCountry: "US", routing.FieldDevice: "mobile"}
	sticky := &routing.StickyState{StreamSetID: "set1", FlowID: "f1-1"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := e.Resolve(ctx(), routing.RequestContext{
			Attributes: attrs,
			Config:     cfg,
			Sticky:     sticky,
			VisitKey:   "bench|1.2.3.4|Mozilla/5.0",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

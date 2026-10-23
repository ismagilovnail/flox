package chstore_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Audit DATA-6: the report builder reads the hourly rollup when it can,
// and must answer exactly what the raw rows answer — for history copied
// by the backfill (before the cutover) and for events counted by the
// views (after it), by day in a time zone, with dimensions and filters,
// unique clicks included.
func TestRollupReportsMatchRawRows(t *testing.T) {
	conn := mustConn(t)
	ctx := context.Background()
	orgID := idgen.New()
	var cutover time.Time
	if err := conn.QueryRow(ctx, `SELECT max(cutover) FROM event_rollup_cutover`).Scan(&cutover); err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(7, 11))
	campaigns := []string{idgen.New(), idgen.New(), idgen.New()}
	pick := func(s ...string) string { return s[r.IntN(len(s))] }
	visitors := []string{"v1", "v2", "v3", "v4", "v5", "v6"}

	// Three days around midnight in Moscow and New York, before and after
	// the cutover.
	periods := []time.Time{
		time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
		cutover.Add(48 * time.Hour).Truncate(24 * time.Hour),
	}
	var evs []event.Event
	for _, start := range periods {
		for i := range 600 {
			at := start.Add(time.Duration(r.IntN(72*60)) * time.Minute).Add(time.Duration(r.IntN(60)) * time.Second)
			e := event.Event{
				ID: idgen.New(), EventAt: at, OrganizationID: orgID, ClickID: idgen.New(),
				CampaignID: pick(campaigns...), TrafficSourceID: pick("ts1", "ts2"), StreamSetID: pick("s1", "s2", ""),
				FlowID: pick("f1", "f2", ""), LandingID: pick("l1", ""), OfferID: pick("o1", "o2", ""),
				NetworkID: pick("n1", ""), Country: pick("US", "DE", "BR", ""), Region: pick("CA", "BY"),
				City: pick("A", "B"), Device: pick("mobile", "desktop"), Platform: pick("android", "ios"),
				OS: pick("Android", "iOS"), Browser: pick("Chrome", "Safari", "Facebook"), Language: pick("en", "de", "pt"),
				IsBot: r.IntN(10) == 0,
			}
			switch i % 10 {
			case 0, 1, 2, 3, 4:
				e.Type = event.SourceClick
				switch r.IntN(3) {
				case 0:
					e.VisitorKey = pick(visitors...) // repeat visitors
				case 1:
					e.IP, e.UserAgent = pick("203.0.113.1", "203.0.113.2"), "UA" // the IP + user agent fallback
				} // else: neither — the click is its own visitor
				if r.IntN(2) == 0 {
					e.Subs = event.Subs{"a"}
				}
			case 5:
				e.Type = event.SourceFilter
			case 6:
				e.Type = event.LandView
			case 7:
				e.Type = event.Type(pick(string(event.PwaInstall), string(event.IosInstall)))
			case 8:
				e.Type = event.TgJoin
				e.TelegramChannelID = pick("tg1", "tg2")
			default:
				e.Type = event.TgStart
			}
			evs = append(evs, e)
		}
	}
	store := chstore.NewEventStore(conn)
	if err := store.InsertBatch(ctx, evs); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	if err := chstore.RunRollupBackfill(ctx, conn, "rollup_test_"+idgen.New(), orgID); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	raw, rolled := chstore.NewEventStore(conn), chstore.NewEventStore(conn)
	raw.ForceRollup(false)
	rolled.ForceRollup(true)

	type query struct {
		name    string
		dims    []chstore.ReportDimension
		byDay   bool
		tz      string
		filters []chstore.ReportFilter
	}
	queries := []query{
		{name: "totals"},
		{name: "country and device by Moscow day", dims: []chstore.ReportDimension{chstore.DimCountry, chstore.DimDevice}, byDay: true, tz: "Europe/Moscow"},
		{name: "campaign by New York day", dims: []chstore.ReportDimension{chstore.DimCampaign}, byDay: true, tz: "America/New_York"},
		{name: "path dims, one campaign", dims: []chstore.ReportDimension{chstore.DimOS, chstore.DimBrowser, chstore.DimLanguage, chstore.DimFlow, chstore.DimLanding, chstore.DimOffer, chstore.DimNetwork, chstore.DimSource, chstore.DimStreamSet},
			filters: []chstore.ReportFilter{{Dimension: chstore.DimCampaign, Value: campaigns[0]}}},
		{name: "telegram channel, two campaigns", dims: []chstore.ReportDimension{chstore.DimTelegramChannel},
			filters: []chstore.ReportFilter{{Dimension: chstore.DimCampaign, In: campaigns[:2]}}},
		{name: "region (not in the rollup)", dims: []chstore.ReportDimension{chstore.DimRegion}},
	}
	for _, start := range periods {
		for _, q := range queries {
			loc := time.UTC
			if q.tz != "" {
				loc, _ = time.LoadLocation(q.tz)
			}
			from := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc).UTC()
			rq := chstore.ReportQuery{OrganizationID: orgID, Dims: q.dims, ByDay: q.byDay, Timezone: q.tz, Filters: q.filters,
				From: from, To: from.Add(96 * time.Hour)}
			name := fmt.Sprintf("%s, %s", q.name, start.Format(time.DateOnly))

			rawClicks, err := raw.ReportClicks(ctx, rq)
			if err != nil {
				t.Fatalf("%s: raw clicks: %v", name, err)
			}
			rolledClicks, err := rolled.ReportClicks(ctx, rq)
			if err != nil {
				t.Fatalf("%s: rollup clicks: %v", name, err)
			}
			if len(rawClicks) == 0 {
				t.Fatalf("%s: no clicks at all — the comparison would be vacuous", name)
			}
			sameRows(t, name+": clicks", rawClicks, rolledClicks, func(a chstore.ClickAgg) chstore.ReportKey { return a.Key })

			rawLand, err := raw.ReportLandViews(ctx, rq)
			if err != nil {
				t.Fatal(err)
			}
			rolledLand, err := rolled.ReportLandViews(ctx, rq)
			if err != nil {
				t.Fatal(err)
			}
			sameRows(t, name+": land views", rawLand, rolledLand, func(a chstore.LandAgg) chstore.ReportKey { return a.Key })

			rawInst, err := raw.ReportPwaInstalls(ctx, rq)
			if err != nil {
				t.Fatal(err)
			}
			rolledInst, err := rolled.ReportPwaInstalls(ctx, rq)
			if err != nil {
				t.Fatal(err)
			}
			sameRows(t, name+": installs", rawInst, rolledInst, func(a chstore.InstallAgg) chstore.ReportKey { return a.Key })
		}
		from := start
		for _, dim := range []chstore.ReportDimension{chstore.DimCountry, chstore.DimBrowser} {
			a, err := raw.DistinctValues(ctx, orgID, dim, from, from.Add(96*time.Hour), 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			b, err := rolled.DistinctValues(ctx, orgID, dim, from, from.Add(96*time.Hour), 10, campaigns)
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(a)
			sort.Strings(b)
			if !reflect.DeepEqual(a, b) {
				t.Errorf("distinct %s from %s: raw %v, rollup %v", dim, from.Format(time.DateOnly), a, b)
			}
		}
	}

	// Both periods really are in the rollup: history through the backfill,
	// the rest through the views.
	for _, start := range periods {
		var n uint64
		if err := conn.QueryRow(ctx, `SELECT count() FROM event_rollup_hourly WHERE organization_id = ? AND event_at >= ? AND event_at < ?`,
			orgID, start, start.Add(96*time.Hour)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("no rollup rows from %s", start.Format(time.DateOnly))
		}
	}
}

// sameRows compares two report answers as sets of rows keyed by their
// group (the row order of a GROUP BY is not defined).
func sameRows[T any](t *testing.T, name string, want, got []T, key func(T) chstore.ReportKey) {
	t.Helper()
	index := func(rows []T) map[string]T {
		out := map[string]T{}
		for _, r := range rows {
			k := key(r)
			s := fmt.Sprint(k.DimValues)
			if k.Day != nil {
				s += k.Day.Format(time.DateOnly)
			}
			out[s] = r
		}
		return out
	}
	w, g := index(want), index(got)
	if len(w) != len(want) || len(g) != len(got) {
		t.Fatalf("%s: duplicate group keys", name)
	}
	if len(w) != len(g) {
		t.Errorf("%s: %d groups from the raw rows, %d from the rollup", name, len(w), len(g))
	}
	for k, wr := range w {
		if gr, ok := g[k]; !ok || !reflect.DeepEqual(wr, gr) {
			t.Errorf("%s: group %s: raw %+v, rollup %+v", name, k, wr, gr)
		}
	}
}

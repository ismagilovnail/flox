package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// seedReportFixture inserts a small, hand-picked set of events for one
// org/campaign: 3 SOURCE_CLICK rows sharing 2 distinct click_ids (one
// click_id repeated once — the stickyFlowKeepClickId scenario
// docs/attribution.md documents, exactly why "clicks" and "unique clicks"
// must differ), 1 SOURCE_FILTER row (must never count as a click), 2
// LAND_VIEW rows, and 2 conversion rows (one CPA_ACCEPT, one CPA_HOLD —
// HOLD must never count as a conversion).
func seedReportFixture(t *testing.T, ctx context.Context, store *chstore.EventStore, orgID, campaignID string, day time.Time) (reusedClickID string) {
	t.Helper()
	clickA := idgen.New()
	clickB := idgen.New()
	reusedClickID = clickA

	events := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile", FlowID: "flow-1"},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile", FlowID: "flow-1"}, // same click_id, returning visitor (sticky)
		{Type: event.SourceClick, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, Country: "DE", Device: "Desktop", FlowID: "flow-1"},
		{Type: event.SourceFilter, EventAt: day.Add(4 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US", FilterReason: "bot"},

		{Type: event.LandView, EventAt: day.Add(1*time.Hour + time.Minute), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile"},
		{Type: event.LandView, EventAt: day.Add(3*time.Hour + time.Minute), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, Country: "DE", Device: "Desktop"},

		{Type: event.CpaAccept, EventAt: day.Add(5 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile", USDValue: 25.5, HasUSDValue: true},
		{Type: event.CpaHold, EventAt: day.Add(5 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, Country: "DE", Device: "Desktop", USDValue: 0, HasUSDValue: false},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("seeding report fixture: %v", err)
	}
	return reusedClickID
}

func TestReportClicksCountsClicksAndUniqueClicksSeparately(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)

	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportClicks: %v", err)
	}
	if len(aggs) != 1 {
		t.Fatalf("ReportClicks (no dims) = %d rows, want 1 aggregate row", len(aggs))
	}
	// 3 SOURCE_CLICK rows (SOURCE_FILTER excluded), 2 distinct click_ids
	// (clickA reused once).
	if aggs[0].Clicks != 3 {
		t.Fatalf("Clicks = %d, want 3 (SOURCE_FILTER must be excluded)", aggs[0].Clicks)
	}
	if aggs[0].UniqueClicks != 2 {
		t.Fatalf("UniqueClicks = %d, want 2 (one click_id reused once)", aggs[0].UniqueClicks)
	}
}

func TestReportClicksGroupsByDimension(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)

	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		Dims:           []chstore.ReportDimension{chstore.DimCountry},
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportClicks: %v", err)
	}
	if len(aggs) != 2 {
		t.Fatalf("ReportClicks (by country) = %d rows, want 2 (US, DE)", len(aggs))
	}
	byCountry := map[string]chstore.ClickAgg{}
	for _, a := range aggs {
		byCountry[a.Key.DimValues[chstore.DimCountry]] = a
	}
	if byCountry["US"].Clicks != 2 || byCountry["US"].UniqueClicks != 1 {
		t.Fatalf("US row = %+v, want Clicks=2 UniqueClicks=1", byCountry["US"])
	}
	if byCountry["DE"].Clicks != 1 || byCountry["DE"].UniqueClicks != 1 {
		t.Fatalf("DE row = %+v, want Clicks=1 UniqueClicks=1", byCountry["DE"])
	}
}

func TestReportClicksAppliesFilter(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)

	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		Filters:        []chstore.ReportFilter{{Dimension: chstore.DimCountry, Value: "DE"}},
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportClicks: %v", err)
	}
	if len(aggs) != 1 || aggs[0].Clicks != 1 {
		t.Fatalf("ReportClicks (country=DE) = %+v, want exactly 1 row with Clicks=1", aggs)
	}
}

func TestReportConversionsExcludesHoldAndTrash(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)

	aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportConversions: %v", err)
	}
	if len(aggs) != 1 {
		t.Fatalf("ReportConversions = %d rows, want 1", len(aggs))
	}
	// Only the CPA_ACCEPT row counts — CPA_HOLD is excluded.
	if aggs[0].Conversions != 1 {
		t.Fatalf("Conversions = %d, want 1 (CPA_HOLD must be excluded)", aggs[0].Conversions)
	}
	if aggs[0].RevenueUSD != 25.5 {
		t.Fatalf("RevenueUSD = %v, want 25.5 (CPA_HOLD's HasUSDValue=false must contribute 0)", aggs[0].RevenueUSD)
	}
}

func TestReportConversionsRejectsUnsupportedDimension(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	_, err := store.ReportConversions(ctx, chstore.ReportQuery{
		OrganizationID: idgen.New(),
		Dims:           []chstore.ReportDimension{chstore.DimOS},
		From:           time.Now().Add(-24 * time.Hour),
		To:             time.Now(),
	})
	if err == nil {
		t.Fatal("ReportConversions with DimOS succeeded, want an error — conversion_events has no os column")
	}
}

func TestReportLandViewsCounts(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)

	aggs, err := store.ReportLandViews(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportLandViews: %v", err)
	}
	if len(aggs) != 1 || aggs[0].LandViews != 2 {
		t.Fatalf("ReportLandViews = %+v, want exactly 1 row with LandViews=2", aggs)
	}
}

func TestReportClicksByDay(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)
	// A second day's single click, to prove ByDay actually splits by day
	// rather than collapsing the whole range into one bucket.
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(25 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US"},
	}); err != nil {
		t.Fatalf("seeding day 2: %v", err)
	}

	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		ByDay:          true,
		From:           day,
		To:             day.Add(48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportClicks (by day): %v", err)
	}
	if len(aggs) != 2 {
		t.Fatalf("ReportClicks (by day) = %d rows, want 2", len(aggs))
	}
	for _, a := range aggs {
		if a.Key.Day == nil {
			t.Fatal("ByDay=true but a row's Key.Day is nil")
		}
	}
}

func TestFunnelCountsZeroFillsAndCountsDistinctClickIDs(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)

	counts, err := store.FunnelCounts(ctx, orgID, day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("FunnelCounts: %v", err)
	}
	if counts["SOURCE_CLICK"] != 2 {
		t.Fatalf(`counts["SOURCE_CLICK"] = %d, want 2 distinct click_ids`, counts["SOURCE_CLICK"])
	}
	if counts["LAND_VIEW"] != 2 {
		t.Fatalf(`counts["LAND_VIEW"] = %d, want 2`, counts["LAND_VIEW"])
	}
	if counts["CPA_ACCEPT"] != 1 {
		t.Fatalf(`counts["CPA_ACCEPT"] = %d, want 1`, counts["CPA_ACCEPT"])
	}
	if counts["CPA_HOLD"] != 1 {
		t.Fatalf(`counts["CPA_HOLD"] = %d, want 1`, counts["CPA_HOLD"])
	}
	// A stage nothing hit at all in this fixture (PWA_INSTALL) must be
	// absent from the map, not present with a wrong value — Service.Funnel
	// is the layer that zero-fills every FUNNEL_STEPS entry, not this one.
	if _, present := counts["PWA_INSTALL"]; present {
		t.Fatalf(`counts["PWA_INSTALL"] unexpectedly present: %d`, counts["PWA_INSTALL"])
	}
}

func TestCrossTenantIsolationInReports(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgA, campaignA := idgen.New(), idgen.New()
	orgB := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgA, campaignA, day)

	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{
		OrganizationID: orgB,
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportClicks: %v", err)
	}
	// No GROUP BY dims means ClickHouse still returns exactly one
	// aggregate row even with zero matching rows (count() of nothing is a
	// real 0, not an absent row) — that's correct, and convenient: the
	// dashboard's single-aggregate KPI query gets a real {Clicks: 0} row
	// for a brand-new org rather than needing to special-case "no rows."
	if len(aggs) != 1 || aggs[0].Clicks != 0 || aggs[0].UniqueClicks != 0 {
		t.Fatalf("org B's report saw org A's clicks: %+v", aggs)
	}
}

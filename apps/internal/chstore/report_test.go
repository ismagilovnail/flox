package chstore_test

import (
	"context"
	"math"
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
// HOLD must never count as a conversion). clickA/clickB each resolve to
// their own stream set (streamSetA/streamSetB below) — conversion.go's
// real ev.StreamSetID = click.StreamSetID copy is simulated by hand here,
// not exercised, since this fixture builds event.Event directly.
func seedReportFixture(t *testing.T, ctx context.Context, store *chstore.EventStore, orgID, campaignID string, day time.Time) (reusedClickID string) {
	t.Helper()
	clickA := idgen.New()
	clickB := idgen.New()
	reusedClickID = clickA
	const streamSetA, streamSetB = "stream-set-a", "stream-set-b"

	events := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile", StreamSetID: streamSetA, FlowID: "flow-1"},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile", StreamSetID: streamSetA, FlowID: "flow-1"}, // same click_id, returning visitor (sticky)
		{Type: event.SourceClick, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, Country: "DE", Device: "Desktop", StreamSetID: streamSetB, FlowID: "flow-1"},
		{Type: event.SourceFilter, EventAt: day.Add(4 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US", FilterReason: "bot"}, // no stream set matched — campaign fallback, StreamSetID stays ""

		{Type: event.LandView, EventAt: day.Add(1*time.Hour + time.Minute), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile"},
		{Type: event.LandView, EventAt: day.Add(3*time.Hour + time.Minute), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, Country: "DE", Device: "Desktop"},

		{Type: event.CpaAccept, EventAt: day.Add(5 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, Country: "US", Device: "Mobile", StreamSetID: streamSetA, USDValue: 25.5, HasUSDValue: true},
		{Type: event.CpaHold, EventAt: day.Add(5 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, Country: "DE", Device: "Desktop", StreamSetID: streamSetB, USDValue: 0, HasUSDValue: false},
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

func TestReportClicksGroupsByStreamSet(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)
	// One more click that matched no stream set at all (campaign fallback)
	// — StreamSetID left as "", the same empty-bucket semantics DimFlow's
	// flow_id already relies on. This must show up as its own "" group,
	// not be dropped or merged into another bucket.
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(6 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US", Device: "Mobile"},
	}); err != nil {
		t.Fatalf("seeding campaign-fallback click: %v", err)
	}

	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		Dims:           []chstore.ReportDimension{chstore.DimStreamSet},
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportClicks (by stream_set): %v", err)
	}
	if len(aggs) != 3 {
		t.Fatalf("ReportClicks (by stream_set) = %d rows, want 3 (stream-set-a, stream-set-b, \"\")", len(aggs))
	}
	byStreamSet := map[string]chstore.ClickAgg{}
	for _, a := range aggs {
		byStreamSet[a.Key.DimValues[chstore.DimStreamSet]] = a
	}
	if byStreamSet["stream-set-a"].Clicks != 2 || byStreamSet["stream-set-a"].UniqueClicks != 1 {
		t.Fatalf("stream-set-a row = %+v, want Clicks=2 UniqueClicks=1", byStreamSet["stream-set-a"])
	}
	if byStreamSet["stream-set-b"].Clicks != 1 || byStreamSet["stream-set-b"].UniqueClicks != 1 {
		t.Fatalf("stream-set-b row = %+v, want Clicks=1 UniqueClicks=1", byStreamSet["stream-set-b"])
	}
	if got, ok := byStreamSet[""]; !ok || got.Clicks != 1 || got.UniqueClicks != 1 {
		t.Fatalf(`"" row (campaign fallback) = %+v (present=%v), want Clicks=1 UniqueClicks=1`, got, ok)
	}
}

func TestReportConversionsGroupsByStreamSetPreservesEmptyBucket(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)
	// An unattributed conversion — no click resolved, so StreamSetID stays
	// "" exactly like FlowID would; this must be its own real group, not
	// missing data.
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.CpaAccept, EventAt: day.Add(6 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US", USDValue: 10, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("seeding unattributed conversion: %v", err)
	}

	aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		Dims:           []chstore.ReportDimension{chstore.DimStreamSet},
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportConversions (by stream_set): %v", err)
	}
	// stream-set-b only has a CPA_HOLD: it is a row now (one registration),
	// but with zero conversions and zero revenue.
	if len(aggs) != 3 {
		t.Fatalf("ReportConversions (by stream_set) = %d rows, want 3 (stream-set-a, stream-set-b, \"\")", len(aggs))
	}
	byStreamSet := map[string]chstore.ConversionAgg{}
	for _, a := range aggs {
		byStreamSet[a.Key.DimValues[chstore.DimStreamSet]] = a
	}
	if byStreamSet["stream-set-a"].Conversions != 1 || byStreamSet["stream-set-a"].RevenueUSD != 25.5 {
		t.Fatalf("stream-set-a row = %+v, want Conversions=1 RevenueUSD=25.5", byStreamSet["stream-set-a"])
	}
	if got := byStreamSet["stream-set-b"]; got.Conversions != 0 || got.RevenueUSD != 0 || got.Registrations != 1 {
		t.Fatalf("stream-set-b row = %+v, want Conversions=0 RevenueUSD=0 Registrations=1 (HOLD only)", got)
	}
	if got, ok := byStreamSet[""]; !ok || got.Conversions != 1 || got.RevenueUSD != 10 {
		t.Fatalf(`"" row (unattributed) = %+v (present=%v), want Conversions=1 RevenueUSD=10`, got, ok)
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
	// ...but the per-status counters still see the HOLD (a registration).
	if aggs[0].Registrations != 1 || aggs[0].Purchases != 1 || aggs[0].Repurchases != 0 {
		t.Fatalf("Registrations/Purchases/Repurchases = %d/%d/%d, want 1/1/0", aggs[0].Registrations, aggs[0].Purchases, aggs[0].Repurchases)
	}
}

func TestReportPwaInstallsCountsAndIsTenantScoped(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, otherOrg, campaignID := idgen.New(), idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.PwaInstall, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US"},
		{Type: event.PwaInstall, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "DE"},
		{Type: event.PwaView, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "DE"},
		{Type: event.PwaInstall, EventAt: day.Add(3 * time.Hour), OrganizationID: otherOrg, CampaignID: campaignID, ClickID: idgen.New(), Country: "US"},
	}); err != nil {
		t.Fatalf("seeding installs: %v", err)
	}
	aggs, err := store.ReportPwaInstalls(ctx, chstore.ReportQuery{
		OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimCountry}, From: day, To: day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportPwaInstalls: %v", err)
	}
	got := map[string]uint64{}
	for _, a := range aggs {
		got[a.Key.DimValues[chstore.DimCountry]] = a.PwaInstalls
	}
	if len(got) != 2 || got["US"] != 1 || got["DE"] != 1 {
		t.Fatalf("installs by country = %v, want US=1 DE=1 (PWA_VIEW and other org excluded)", got)
	}
}

func TestReportConversionsCountsRedepsSeparately(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID, click := idgen.New(), idgen.New(), idgen.New()
	declined, trashed := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day, OrganizationID: orgID, CampaignID: campaignID, ClickID: click, OS: "linux"},
		// A registration the network then rejected, and a junk lead.
		{Type: event.SourceClick, EventAt: day, OrganizationID: orgID, CampaignID: campaignID, ClickID: declined, OS: "linux"},
		{Type: event.CpaHold, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: declined},
		{Type: event.CpaDecline, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: declined},
		{Type: event.SourceClick, EventAt: day, OrganizationID: orgID, CampaignID: campaignID, ClickID: trashed, OS: "linux"},
		{Type: event.CpaTrash, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: trashed},
		{Type: event.CpaHold, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click},
		{Type: event.CpaAccept, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, USDValue: 10, HasUSDValue: true},
		{Type: event.CpaRedep, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, USDValue: 5, HasUSDValue: true, EventRef: "t1"},
		{Type: event.CpaRedep, EventAt: day.Add(4 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, USDValue: 5, HasUSDValue: true, EventRef: "t2"},
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	// Plain and click-joined (by OS) paths must agree.
	for name, dims := range map[string][]chstore.ReportDimension{"plain": nil, "joined": {chstore.DimOS}} {
		aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{OrganizationID: orgID, Dims: dims, From: day, To: day.Add(24 * time.Hour)})
		if err != nil || len(aggs) != 1 {
			t.Fatalf("%s: ReportConversions = %+v, %v", name, aggs, err)
		}
		a := aggs[0]
		if a.Registrations != 2 || a.Purchases != 1 || a.Repurchases != 2 || a.Declines != 1 || a.Trash != 1 || a.Conversions != 3 || a.RevenueUSD != 20 {
			t.Fatalf("%s: got %+v, want reg=2 purchases=1 redeps=2 declines=1 trash=1 conversions=3 revenue=20", name, a)
		}
	}
}

// seedOSJoinFixture: two clicks (linux/Chrome, windows/Firefox), the linux
// one a day BEFORE the report window — a conversion can land days after its
// click, so the join must not bound clicks by the report range. Plus a
// conversion whose click was never recorded.
func seedOSJoinFixture(t *testing.T, ctx context.Context, store *chstore.EventStore, orgID, campaignID string, day time.Time) {
	t.Helper()
	linuxClick, winClick := idgen.New(), idgen.New()
	events := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(-20 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: linuxClick, OS: "linux", Browser: "Chrome", Language: "en"},
		{Type: event.SourceClick, EventAt: day.Add(-19 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: linuxClick, OS: "linux", Browser: "Chrome", Language: "en"}, // sticky repeat: must not double-count the conversion
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: winClick, OS: "windows", Browser: "Firefox", Language: "ru"},
		{Type: event.CpaAccept, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: linuxClick, USDValue: 30, HasUSDValue: true},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: winClick, USDValue: 12, HasUSDValue: true},
		{Type: event.CpaAccept, EventAt: day.Add(4 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), USDValue: 5, HasUSDValue: true}, // no click on record
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("seeding os-join fixture: %v", err)
	}
}

func TestReportConversionsGroupsByOSViaClickJoin(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	seedOSJoinFixture(t, ctx, store, orgID, campaignID, day)

	aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		Dims:           []chstore.ReportDimension{chstore.DimOS},
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportConversions (by os): %v", err)
	}
	byOS := map[string]chstore.ConversionAgg{}
	for _, a := range aggs {
		byOS[a.Key.DimValues[chstore.DimOS]] = a
	}
	if len(byOS) != 3 {
		t.Fatalf("rows = %+v, want linux, windows and the unattributed \"\" bucket", byOS)
	}
	if got := byOS["linux"]; got.Conversions != 1 || got.RevenueUSD != 30 {
		t.Fatalf("linux = %+v, want 1 conversion / 30 (click is outside the window and sticky-repeated)", got)
	}
	if got := byOS["windows"]; got.Conversions != 1 || got.RevenueUSD != 12 {
		t.Fatalf("windows = %+v, want 1 / 12", got)
	}
	if got := byOS[""]; got.Conversions != 1 || got.RevenueUSD != 5 {
		t.Fatalf(`"" = %+v, want 1 / 5 (no click on record)`, got)
	}
}

func TestReportConversionsFiltersByBrowserViaClickJoin(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	seedOSJoinFixture(t, ctx, store, orgID, campaignID, day)

	aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{
		OrganizationID: orgID,
		Dims:           []chstore.ReportDimension{chstore.DimCampaign},
		Filters:        []chstore.ReportFilter{{Dimension: chstore.DimBrowser, Value: "Firefox"}},
		From:           day,
		To:             day.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("ReportConversions (filter browser): %v", err)
	}
	if len(aggs) != 1 || aggs[0].Conversions != 1 || aggs[0].RevenueUSD != 12 {
		t.Fatalf("aggs = %+v, want one campaign row with 1 conversion / 12", aggs)
	}
}

func TestReportConversionsViaClickJoinByDayAndTenantIsolation(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgA, orgB, campaignID := idgen.New(), idgen.New(), idgen.New()
	day := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	seedOSJoinFixture(t, ctx, store, orgA, campaignID, day)

	// Org B has a conversion carrying the SAME click_id as one of org A's
	// clicks — it must neither pick up A's os nor show up in A's report.
	var clickID string
	rows, err := conn.Query(ctx, "SELECT click_id FROM click_events WHERE organization_id = ? AND os = 'linux' LIMIT 1", orgA)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		_ = rows.Scan(&clickID)
	}
	rows.Close()
	if clickID == "" {
		t.Fatal("fixture click not found")
	}
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.CpaAccept, EventAt: day.Add(2 * time.Hour), OrganizationID: orgB, CampaignID: campaignID, ClickID: clickID, USDValue: 999, HasUSDValue: true},
	}); err != nil {
		t.Fatal(err)
	}

	for _, org := range []string{orgA, orgB} {
		aggs, err := store.ReportConversions(ctx, chstore.ReportQuery{
			OrganizationID: org,
			Dims:           []chstore.ReportDimension{chstore.DimOS},
			ByDay:          true,
			From:           day,
			To:             day.Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("ReportConversions (by os, by day) org %s: %v", org, err)
		}
		var linux, total float64
		for _, a := range aggs {
			total += a.RevenueUSD
			if a.Key.DimValues[chstore.DimOS] == "linux" {
				linux += a.RevenueUSD
			}
		}
		if org == orgA && (linux != 30 || total != 47) {
			t.Fatalf("org A: linux=%v total=%v, want 30 / 47 and no revenue from org B", linux, total)
		}
		if org == orgB && (linux != 0 || total != 999) {
			t.Fatalf("org B: linux=%v total=%v, want 0 / 999 (org A's click must not leak its os)", linux, total)
		}
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

	counts, err := store.FunnelCounts(ctx, orgID, day, day.Add(24*time.Hour), nil)
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

// TestDistinctValuesOrdersByFrequencyAndExcludesEmpty is the live-
// ClickHouse verification behind the report builder's filter-value
// dropdown (docs/analytics-report-api.md) — real distinct values, most-
// common first, with the DimFlow/DimStreamSet empty bucket deliberately
// left out of a value list meant to be clicked, not typed.
func TestDistinctValuesOrdersByFrequencyAndExcludesEmpty(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day)
	// A third, real SOURCE_CLICK matching no stream set (campaign
	// fallback) — StreamSetID stays "" even though this click counts,
	// proving DistinctValues excludes the empty bucket rather than
	// surfacing it as an unlabeled dropdown entry. Also a third US
	// click, so country's frequency order is unambiguous (US=3, DE=1).
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(6 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US"},
	}); err != nil {
		t.Fatalf("seeding campaign-fallback click: %v", err)
	}

	countryValues, err := store.DistinctValues(ctx, orgID, chstore.DimCountry, day, day.Add(24*time.Hour), 10)
	if err != nil {
		t.Fatalf("DistinctValues (country): %v", err)
	}
	if len(countryValues) != 2 || countryValues[0] != "US" || countryValues[1] != "DE" {
		t.Fatalf("country values = %v, want [US DE] ordered by frequency (US=3, DE=1)", countryValues)
	}

	streamSetValues, err := store.DistinctValues(ctx, orgID, chstore.DimStreamSet, day, day.Add(24*time.Hour), 10)
	if err != nil {
		t.Fatalf("DistinctValues (stream_set): %v", err)
	}
	if len(streamSetValues) != 2 {
		t.Fatalf("stream_set values = %v, want exactly 2 (stream-set-a, stream-set-b) — the fallback click's \"\" must be excluded, not a third entry", streamSetValues)
	}
	for _, v := range streamSetValues {
		if v == "" {
			t.Fatal(`DistinctValues returned "" as a pickable value, want the empty bucket excluded`)
		}
	}
}

// TestDistinctValuesLimitsResults confirms the limit argument actually
// caps ClickHouse's own LIMIT clause, not just something the caller
// truncates to after the fact.
func TestDistinctValuesLimitsResults(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	seedReportFixture(t, ctx, store, orgID, campaignID, day) // seeds 2 distinct countries: US, DE

	values, err := store.DistinctValues(ctx, orgID, chstore.DimCountry, day, day.Add(24*time.Hour), 1)
	if err != nil {
		t.Fatalf("DistinctValues: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("values = %v, want exactly 1 (limit=1 against 2 real distinct countries)", values)
	}
}

func TestDistinctValuesRejectsUnsupportedDimension(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	_, err := store.DistinctValues(context.Background(), idgen.New(), chstore.ReportDimension("no_such_dimension"), time.Now().Add(-24*time.Hour), time.Now(), 10)
	if err == nil {
		t.Fatal(`DistinctValues with an unknown dimension succeeded, want an error`)
	}
}

// TestReportByLanding covers schema/008's landing_id end to end through
// InsertBatch (all three tables' positional appends) and every report
// query: clicks, land views, conversions — each with its own "" bucket
// for traffic that was shown no landing.
func TestReportByLanding(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	clickA, clickB, clickC := idgen.New(), idgen.New(), idgen.New()
	const lndA, lndB = "landing-a", "landing-b"

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, LandingID: lndA},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, LandingID: lndB},
		{Type: event.SourceClick, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickC}, // no landing stage
		{Type: event.LandView, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, LandingID: lndA},
		{Type: event.LandView, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, LandingID: lndB},
		{Type: event.LandClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, LandingID: lndB},
		{Type: event.CpaAccept, EventAt: day.Add(4 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB, LandingID: lndB, USDValue: 40, HasUSDValue: true},
		{Type: event.CpaAccept, EventAt: day.Add(4 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickC, USDValue: 5, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("InsertBatch with landing ids: %v", err)
	}
	q := chstore.ReportQuery{OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimLanding}, From: day, To: day.Add(24 * time.Hour)}

	clicks, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks by landing: %v", err)
	}
	gotClicks := map[string]uint64{}
	for _, a := range clicks {
		gotClicks[a.Key.DimValues[chstore.DimLanding]] = a.Clicks
	}
	if len(gotClicks) != 3 || gotClicks[lndA] != 1 || gotClicks[lndB] != 1 || gotClicks[""] != 1 {
		t.Fatalf("clicks by landing = %v, want landing-a:1 landing-b:1 \"\":1", gotClicks)
	}

	views, err := store.ReportLandViews(ctx, q)
	if err != nil {
		t.Fatalf("ReportLandViews by landing: %v", err)
	}
	gotViews := map[string]uint64{}
	for _, a := range views {
		gotViews[a.Key.DimValues[chstore.DimLanding]] = a.LandViews
	}
	if len(gotViews) != 2 || gotViews[lndA] != 1 || gotViews[lndB] != 1 {
		t.Fatalf("land views by landing = %v, want landing-a:1 landing-b:1", gotViews)
	}

	convs, err := store.ReportConversions(ctx, q)
	if err != nil {
		t.Fatalf("ReportConversions by landing: %v", err)
	}
	gotRevenue := map[string]float64{}
	for _, a := range convs {
		gotRevenue[a.Key.DimValues[chstore.DimLanding]] = a.RevenueUSD
	}
	if len(gotRevenue) != 2 || gotRevenue[lndB] != 40 || gotRevenue[""] != 5 {
		t.Fatalf("revenue by landing = %v, want landing-b:40 \"\":5", gotRevenue)
	}

	values, err := store.DistinctValues(ctx, orgID, chstore.DimLanding, day, day.Add(24*time.Hour), 10)
	if err != nil {
		t.Fatalf("DistinctValues(landing): %v", err)
	}
	seen := map[string]bool{}
	for _, v := range values {
		seen[v] = true
	}
	if !seen[lndA] || !seen[lndB] {
		t.Fatalf("DistinctValues(landing) = %v, want both landing ids", values)
	}
}

// TestReportByOffer: offer_id (schema/009) groups clicks and conversions,
// with "" as its own bucket (redirect flows / fallbacks).
func TestReportByOffer(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	clickA, clickB := idgen.New(), idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, OfferID: "offer-a", LandingID: "landing-a"},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickB},
		{Type: event.LandClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, OfferID: "offer-a", LandingID: "landing-a"},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickA, OfferID: "offer-a", USDValue: 30, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("InsertBatch with offer ids: %v", err)
	}
	q := chstore.ReportQuery{OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimOffer, chstore.DimLanding}, From: day, To: day.Add(24 * time.Hour)}

	clicks, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks by offer+landing: %v", err)
	}
	got := map[string]uint64{}
	for _, a := range clicks {
		got[a.Key.DimValues[chstore.DimOffer]+"|"+a.Key.DimValues[chstore.DimLanding]] = a.Clicks
	}
	if len(got) != 2 || got["offer-a|landing-a"] != 1 || got["|"] != 1 {
		t.Fatalf("clicks by offer|landing = %v, want offer-a|landing-a:1 and |:1", got)
	}

	convs, err := store.ReportConversions(ctx, chstore.ReportQuery{OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimOffer}, From: day, To: day.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("ReportConversions by offer: %v", err)
	}
	if len(convs) != 1 || convs[0].Key.DimValues[chstore.DimOffer] != "offer-a" || convs[0].RevenueUSD != 30 {
		t.Fatalf("conversions by offer = %+v, want offer-a with $30", convs)
	}
}

// TestReportByPwa: pwa_id (schema/010) groups clicks, PWA funnel events
// and conversions.
func TestReportByPwa(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	click := idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PwaID: "pwa-a"},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.PwaView, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PwaID: "pwa-a"},
		{Type: event.PwaInstall, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PwaID: "pwa-a"},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PwaID: "pwa-a", USDValue: 12, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("InsertBatch with pwa ids: %v", err)
	}
	q := chstore.ReportQuery{OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimPwa}, From: day, To: day.Add(24 * time.Hour)}
	clicks, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks by pwa: %v", err)
	}
	got := map[string]uint64{}
	for _, a := range clicks {
		got[a.Key.DimValues[chstore.DimPwa]] = a.Clicks
	}
	if len(got) != 2 || got["pwa-a"] != 1 || got[""] != 1 {
		t.Fatalf("clicks by pwa = %v", got)
	}
	convs, err := store.ReportConversions(ctx, q)
	if err != nil || len(convs) != 1 || convs[0].RevenueUSD != 12 {
		t.Fatalf("conversions by pwa = %+v, %v", convs, err)
	}
}

// TestReportByPostlanding: postlanding_id (schema/016) groups clicks, the
// stage's own events and conversions.
func TestReportByPostlanding(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	click := idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PostlandingID: "post-a"},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.PostlandingView, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PostlandingID: "post-a"},
		{Type: event.PostlandingClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PostlandingID: "post-a"},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, PostlandingID: "post-a", USDValue: 7, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("InsertBatch with postlanding ids: %v", err)
	}
	q := chstore.ReportQuery{OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimPostlanding}, From: day, To: day.Add(24 * time.Hour)}
	clicks, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks by postlanding: %v", err)
	}
	got := map[string]uint64{}
	for _, a := range clicks {
		got[a.Key.DimValues[chstore.DimPostlanding]] = a.Clicks
	}
	if len(got) != 2 || got["post-a"] != 1 || got[""] != 1 {
		t.Fatalf("clicks by postlanding = %v", got)
	}
	convs, err := store.ReportConversions(ctx, q)
	if err != nil || len(convs) != 1 || convs[0].Key.DimValues[chstore.DimPostlanding] != "post-a" || convs[0].RevenueUSD != 7 {
		t.Fatalf("conversions by postlanding = %+v, %v", convs, err)
	}
}

// TestReportBySourceMergesCampaignsOnOneSource covers schema/011's
// traffic_source_id end to end: two campaigns on the same source collapse
// into ONE row (clicks and conversions), a campaign on another source stays
// separate, and rows with no source land in the "" bucket.
func TestReportBySourceMergesCampaignsOnOneSource(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID := idgen.New()
	campA, campB, campC := idgen.New(), idgen.New(), idgen.New()
	const srcFB, srcTT = "source-facebook", "source-tiktok"
	day := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	clickA, clickB, clickC := idgen.New(), idgen.New(), idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campA, ClickID: clickA, TrafficSourceID: srcFB},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campB, ClickID: clickB, TrafficSourceID: srcFB},
		{Type: event.SourceClick, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campC, ClickID: clickC, TrafficSourceID: srcTT},
		{Type: event.SourceClick, EventAt: day.Add(4 * time.Hour), OrganizationID: orgID, CampaignID: campC, ClickID: idgen.New()}, // pre-011 row, never backfilled
		{Type: event.CpaAccept, EventAt: day.Add(5 * time.Hour), OrganizationID: orgID, CampaignID: campA, ClickID: clickA, TrafficSourceID: srcFB, USDValue: 10, HasUSDValue: true},
		{Type: event.CpaAccept, EventAt: day.Add(6 * time.Hour), OrganizationID: orgID, CampaignID: campB, ClickID: clickB, TrafficSourceID: srcFB, USDValue: 15, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	q := chstore.ReportQuery{OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimSource}, From: day, To: day.Add(24 * time.Hour)}
	clicks, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks: %v", err)
	}
	bySource := map[string]uint64{}
	for _, c := range clicks {
		bySource[c.Key.DimValues[chstore.DimSource]] = c.Clicks
	}
	if len(bySource) != 3 || bySource[srcFB] != 2 || bySource[srcTT] != 1 || bySource[""] != 1 {
		t.Fatalf("clicks by source = %v, want facebook=2 (two campaigns merged), tiktok=1, \"\"=1", bySource)
	}

	convs, err := store.ReportConversions(ctx, q)
	if err != nil {
		t.Fatalf("ReportConversions: %v", err)
	}
	if len(convs) != 1 || convs[0].Key.DimValues[chstore.DimSource] != srcFB || convs[0].Conversions != 2 || convs[0].RevenueUSD != 25 {
		t.Fatalf("conversions by source = %+v, want one facebook row with 2 / 25", convs)
	}
}

func TestReportClicksByDayCutsDaysInTheRequestedTimezone(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	// 23:30Z and 01:30Z the next UTC day: two UTC days, but both fall on
	// 11 Aug in Dubai (UTC+4).
	late := time.Date(2026, 8, 10, 23, 30, 0, 0, time.UTC)
	early := time.Date(2026, 8, 11, 1, 30, 0, 0, time.UTC)
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: late, OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.SourceClick, EventAt: early, OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	q := chstore.ReportQuery{OrganizationID: orgID, ByDay: true, From: late.Add(-time.Hour), To: early.Add(time.Hour)}

	utc, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks (UTC): %v", err)
	}
	if len(utc) != 2 {
		t.Fatalf("UTC days = %d, want 2 (10 Aug and 11 Aug)", len(utc))
	}

	q.Timezone = "Asia/Dubai"
	dubai, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks (Dubai): %v", err)
	}
	if len(dubai) != 1 || dubai[0].Clicks != 2 || dubai[0].Key.Day.Format("2006-01-02") != "2026-08-11" {
		t.Fatalf("Dubai rows = %+v, want one 11 Aug row with 2 clicks", dubai)
	}

	q.Timezone = "x'; DROP TABLE click_events; --"
	if _, err := store.ReportClicks(ctx, q); err != nil {
		t.Fatalf("an invalid zone name must fall back to UTC, not reach the query: %v", err)
	}
}

func TestFunnelCountsAppliesFiltersToEveryStage(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID := idgen.New()
	campA, campB := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	a1, a2, b1 := idgen.New(), idgen.New(), idgen.New()
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campA, ClickID: a1, OS: "linux"},
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campA, ClickID: a2, OS: "windows"},
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campB, ClickID: b1, OS: "linux"},
		{Type: event.LandView, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campA, ClickID: a1},
		{Type: event.LandView, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campB, ClickID: b1},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campA, ClickID: a1, USDValue: 10, HasUSDValue: true},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campB, ClickID: b1, USDValue: 10, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	from, to := day, day.Add(24*time.Hour)

	all, err := store.FunnelCounts(ctx, orgID, from, to, nil)
	if err != nil {
		t.Fatalf("FunnelCounts: %v", err)
	}
	if all["SOURCE_CLICK"] != 3 || all["LAND_VIEW"] != 2 || all["CPA_ACCEPT"] != 2 {
		t.Fatalf("unfiltered funnel = %v, want clicks 3 / land views 2 / accepts 2", all)
	}

	// Campaign filter reaches the tracking and conversion tables through the click.
	byCamp, err := store.FunnelCounts(ctx, orgID, from, to, []chstore.ReportFilter{{Dimension: chstore.DimCampaign, Value: campA}})
	if err != nil {
		t.Fatalf("FunnelCounts (campaign): %v", err)
	}
	if byCamp["SOURCE_CLICK"] != 2 || byCamp["LAND_VIEW"] != 1 || byCamp["CPA_ACCEPT"] != 1 {
		t.Fatalf("campaign A funnel = %v, want 2 / 1 / 1", byCamp)
	}

	// os exists only on clicks — the conversion stage must still honour it.
	byOS, err := store.FunnelCounts(ctx, orgID, from, to, []chstore.ReportFilter{{Dimension: chstore.DimCampaign, Value: campA}, {Dimension: chstore.DimOS, Value: "linux"}})
	if err != nil {
		t.Fatalf("FunnelCounts (campaign+os): %v", err)
	}
	if byOS["SOURCE_CLICK"] != 1 || byOS["LAND_VIEW"] != 1 || byOS["CPA_ACCEPT"] != 1 {
		t.Fatalf("campaign A + linux funnel = %v, want 1 / 1 / 1", byOS)
	}

	// Another org sees none of it.
	other, err := store.FunnelCounts(ctx, idgen.New(), from, to, []chstore.ReportFilter{{Dimension: chstore.DimCampaign, Value: campA}})
	if err != nil {
		t.Fatalf("FunnelCounts (other org): %v", err)
	}
	if other["SOURCE_CLICK"] != 0 || other["CPA_ACCEPT"] != 0 {
		t.Fatalf("other org funnel = %v, want empty", other)
	}
}

// Two visits by the same visitor mint two click_ids, but are one unique click;
// a different IP or user agent is another visitor, and a click with neither
// falls back to its own click_id.
func TestReportClicksUniqueIsPerVisitorNotPerClickID(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	click := func(h int, ip, ua string) event.Event {
		return event.Event{Type: event.SourceClick, EventAt: day.Add(time.Duration(h) * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), IP: ip, UserAgent: ua}
	}
	if err := store.InsertBatch(ctx, []event.Event{
		click(1, "203.0.113.7", "Mozilla/5.0 A"),
		click(2, "203.0.113.7", "Mozilla/5.0 A"),  // same visitor again
		click(3, "203.0.113.7", "Mozilla/5.0 B"),  // same IP, other browser
		click(4, "198.51.100.1", "Mozilla/5.0 A"), // other IP
		click(5, "", ""),
		click(6, "", ""), // no fingerprint: each counts on its own
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{OrganizationID: orgID, From: day, To: day.Add(24 * time.Hour)})
	if err != nil || len(aggs) != 1 {
		t.Fatalf("ReportClicks = %+v, %v", aggs, err)
	}
	if aggs[0].Clicks != 6 || aggs[0].UniqueClicks != 5 {
		t.Fatalf("Clicks/UniqueClicks = %d/%d, want 6/5", aggs[0].Clicks, aggs[0].UniqueClicks)
	}
}

func TestReportClicksCountsBlockedAndBotsWithoutTouchingClicks(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, otherOrg, campaignID := idgen.New(), idgen.New(), idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	at := func(h int) time.Time { return day.Add(time.Duration(h) * time.Hour) }
	events := []event.Event{
		{Type: event.SourceClick, EventAt: at(1), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US"},
		{Type: event.SourceClick, EventAt: at(2), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US", IsBot: true}, // bot, still redirected
		{Type: event.SourceFilter, EventAt: at(3), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US", IsBot: true},
		{Type: event.SourceFilter, EventAt: at(4), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "DE"}, // blocked by geo, not a bot
		// another organization's traffic must never leak in
		{Type: event.SourceFilter, EventAt: at(3), OrganizationID: otherOrg, CampaignID: campaignID, ClickID: idgen.New(), Country: "US", IsBot: true},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{
		OrganizationID: orgID, From: day, To: day.Add(24 * time.Hour),
		Dims: []chstore.ReportDimension{chstore.DimCountry},
	})
	if err != nil {
		t.Fatalf("ReportClicks: %v", err)
	}
	got := map[string]chstore.ClickAgg{}
	for _, a := range aggs {
		got[a.Key.DimValues[chstore.DimCountry]] = a
	}
	us, de := got["US"], got["DE"]
	if us.Clicks != 2 || us.Blocked != 1 || us.Bots != 2 {
		t.Fatalf("US = clicks %d blocked %d bots %d, want 2/1/2", us.Clicks, us.Blocked, us.Bots)
	}
	// A country with only blocked traffic still appears, with 0 clicks.
	if de.Clicks != 0 || de.Blocked != 1 || de.Bots != 0 {
		t.Fatalf("DE = clicks %d blocked %d bots %d, want 0/1/0", de.Clicks, de.Blocked, de.Bots)
	}
}

// TestReportByNetwork: clicks group by the network of the offer they were
// routed to (schema/017), conversions by the network that sent the
// postback (schema/003) — one `network` dimension across both.
func TestReportByNetwork(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	click := idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, OfferID: "offer-a", NetworkID: "net-a"},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.LandView, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, NetworkID: "net-a"},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, NetworkID: "net-a", USDValue: 9, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("InsertBatch with network ids: %v", err)
	}
	q := chstore.ReportQuery{OrganizationID: orgID, Dims: []chstore.ReportDimension{chstore.DimNetwork}, From: day, To: day.Add(24 * time.Hour)}
	clicks, err := store.ReportClicks(ctx, q)
	if err != nil {
		t.Fatalf("ReportClicks by network: %v", err)
	}
	got := map[string]uint64{}
	for _, a := range clicks {
		got[a.Key.DimValues[chstore.DimNetwork]] = a.Clicks
	}
	if len(got) != 2 || got["net-a"] != 1 || got[""] != 1 {
		t.Fatalf("clicks by network = %v", got)
	}
	convs, err := store.ReportConversions(ctx, q)
	if err != nil || len(convs) != 1 || convs[0].Key.DimValues[chstore.DimNetwork] != "net-a" || convs[0].RevenueUSD != 9 {
		t.Fatalf("conversions by network = %+v, %v", convs, err)
	}
}

func TestReportConversionsInReportingCurrency(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID, click := idgen.New(), idgen.New(), idgen.New()
	d1 := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	d2 := d1.AddDate(0, 0, 1)
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: d1, OrganizationID: orgID, CampaignID: campaignID, ClickID: click, OS: "linux"},
		// 23:30 UTC is still d1 for the rate, whatever zone the report is in.
		{Type: event.CpaAccept, EventAt: d1.Add(23*time.Hour + 30*time.Minute), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, USDValue: 10, HasUSDValue: true},
		{Type: event.CpaRedep, EventAt: d2.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, USDValue: 4, HasUSDValue: true, EventRef: "r1"},
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	q := chstore.ReportQuery{OrganizationID: orgID, From: d1, To: d2.Add(24 * time.Hour),
		Money: &chstore.MoneyFactors{Days: []time.Time{d1, d2}, Factors: []float64{0.5, 0.25}}}
	for name, dims := range map[string][]chstore.ReportDimension{"plain": nil, "joined": {chstore.DimOS}} {
		q.Dims = dims
		aggs, err := store.ReportConversions(ctx, q)
		if err != nil || len(aggs) != 1 || aggs[0].RevenueUSD != 6 { // 10*0.5 + 4*0.25
			t.Fatalf("%s: ReportConversions = %+v, %v, want revenue 6", name, aggs, err)
		}
	}
	// d2 without a rate: the total is unknown (NaN), never a partial sum.
	q.Dims = nil
	q.Money = &chstore.MoneyFactors{Days: []time.Time{d1}, Factors: []float64{0.5}}
	aggs, err := store.ReportConversions(ctx, q)
	if err != nil || len(aggs) != 1 || !math.IsNaN(aggs[0].RevenueUSD) {
		t.Fatalf("ReportConversions = %+v, %v, want NaN revenue", aggs, err)
	}
}

// TestFlowStageCountsPerFlowAndTenant is the live-ClickHouse check behind
// the flow builder's node summaries (§25): per flow, distinct visits for
// the page stages (a reload is not a second view), every CPA_REDEP as its
// own conversion, and nothing from a flow not asked for, outside the
// range, or from another organization.
func TestFlowStageCountsPerFlowAndTenant(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, otherOrg, campaignID := idgen.New(), idgen.New(), idgen.New()
	flowA, flowB, flowC := idgen.New(), idgen.New(), idgen.New()
	day := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	click1, click2, click3 := idgen.New(), idgen.New(), idgen.New()
	ev := func(typ event.Type, at time.Duration, org, click, flow string) event.Event {
		return event.Event{Type: typ, EventAt: day.Add(at), OrganizationID: org, CampaignID: campaignID, ClickID: click, FlowID: flow}
	}

	events := []event.Event{
		ev(event.SourceClick, 1*time.Hour, orgID, click1, flowA),
		ev(event.SourceClick, 2*time.Hour, orgID, click2, flowA),
		ev(event.LandView, 1*time.Hour+time.Minute, orgID, click1, flowA),
		ev(event.LandView, 1*time.Hour+2*time.Minute, orgID, click1, flowA), // reload
		ev(event.LandView, 2*time.Hour+time.Minute, orgID, click2, flowA),
		ev(event.LandClick, 1*time.Hour+3*time.Minute, orgID, click1, flowA),
		ev(event.CpaAccept, 5*time.Hour, orgID, click1, flowA),
		ev(event.SourceClick, 3*time.Hour, orgID, click3, flowB),
		ev(event.SourceClick, 3*time.Hour, orgID, idgen.New(), flowC),    // flow not asked for
		ev(event.SourceClick, 25*time.Hour, orgID, idgen.New(), flowA),   // after the range
		ev(event.SourceClick, 1*time.Hour, otherOrg, idgen.New(), flowA), // another organization, same flow id
		ev(event.LandView, 1*time.Hour, otherOrg, idgen.New(), flowA),
	}
	redep := func(at time.Duration, txn string) event.Event {
		e := ev(event.CpaRedep, at, orgID, click1, flowA)
		e.NetworkTxnID, e.EventRef = txn, txn
		return e
	}
	events = append(events, redep(6*time.Hour, "txn-1"), redep(7*time.Hour, "txn-2"))
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	counts, err := store.FlowStageCounts(ctx, orgID, day, day.Add(24*time.Hour), []string{flowA, flowB})
	if err != nil {
		t.Fatalf("FlowStageCounts: %v", err)
	}
	wantA := map[string]uint64{"SOURCE_CLICK": 2, "LAND_VIEW": 2, "LAND_CLICK": 1, "CPA_ACCEPT": 1, "CPA_REDEP": 2}
	for typ, want := range wantA {
		if got := counts[flowA][typ]; got != want {
			t.Errorf("flow A %s = %d, want %d", typ, got, want)
		}
	}
	if len(counts[flowA]) != len(wantA) {
		t.Errorf("flow A = %v, want only %v (absent stages stay absent)", counts[flowA], wantA)
	}
	if counts[flowB]["SOURCE_CLICK"] != 1 {
		t.Errorf("flow B SOURCE_CLICK = %d, want 1", counts[flowB]["SOURCE_CLICK"])
	}
	if _, ok := counts[flowC]; ok {
		t.Errorf("flow C was not asked for but came back: %v", counts[flowC])
	}

	other, err := store.FlowStageCounts(ctx, otherOrg, day, day.Add(24*time.Hour), []string{flowA})
	if err != nil {
		t.Fatalf("FlowStageCounts (other org): %v", err)
	}
	if other[flowA]["SOURCE_CLICK"] != 1 || other[flowA]["LAND_VIEW"] != 1 || other[flowA]["CPA_REDEP"] != 0 {
		t.Errorf("other org sees %v, want only its own click and view", other[flowA])
	}
}

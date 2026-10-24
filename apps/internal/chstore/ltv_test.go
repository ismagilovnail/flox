package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestClicksByFTDAnchor(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	campaignID := idgen.New()
	networkID := idgen.New()
	click1 := idgen.New()
	click2 := idgen.New() // outside the anchor window

	ftdDay := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	err := store.InsertBatch(ctx, []event.Event{
		{Type: event.CpaHold, EventAt: ftdDay.Add(-24 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, NetworkID: networkID, ClickID: click1, Country: "US"},
		{Type: event.CpaAccept, EventAt: ftdDay, OrganizationID: orgID, CampaignID: campaignID, NetworkID: networkID, ClickID: click1, Country: "US", Revenue: 50, USDValue: 50, HasUSDValue: true},
		{Type: event.CpaRedep, EventAt: ftdDay.Add(5 * 24 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, NetworkID: networkID, ClickID: click1, Country: "US", Revenue: 20, USDValue: 20, HasUSDValue: true, NetworkTxnID: "t1"},
		// click2's FTD is a year later — must not appear in a query scoped to January.
		{Type: event.CpaAccept, EventAt: ftdDay.AddDate(1, 0, 0), OrganizationID: orgID, CampaignID: campaignID, NetworkID: networkID, ClickID: click2, Country: "US", Revenue: 10, USDValue: 10, HasUSDValue: true},
	})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	histories, err := store.ClicksByFTDAnchor(ctx, orgID, chstore.LTVFilter{
		From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ClicksByFTDAnchor: %v", err)
	}
	if len(histories) != 1 {
		t.Fatalf("histories = %d, want 1 (only click1's FTD falls in January)", len(histories))
	}
	h := histories[0]
	if h.ClickID != click1 || h.CampaignID != campaignID || h.NetworkID != networkID || h.Country != "US" {
		t.Fatalf("history = %+v, want matching click1", h)
	}
	// Full history: HOLD + ACCEPT + REDEP, even though the REDEP is 5 days
	// past the anchor window's own end.
	if len(h.Deposits) != 3 {
		t.Fatalf("deposits = %d, want 3 (HOLD, ACCEPT, REDEP)", len(h.Deposits))
	}
	types := map[event.Type]bool{}
	for _, d := range h.Deposits {
		types[d.Type] = true
	}
	if !types[event.CpaHold] || !types[event.CpaAccept] || !types[event.CpaRedep] {
		t.Fatalf("deposit types = %v, want all three", types)
	}
}

func TestClicksByRegAnchor(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	clickID := idgen.New()
	regDay := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.CpaHold, EventAt: regDay, OrganizationID: orgID, ClickID: clickID},
		{Type: event.CpaAccept, EventAt: regDay.Add(48 * time.Hour), OrganizationID: orgID, ClickID: clickID, USDValue: 30, HasUSDValue: true},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	histories, err := store.ClicksByRegAnchor(ctx, orgID, chstore.LTVFilter{
		From: regDay.Add(-time.Hour), To: regDay.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ClicksByRegAnchor: %v", err)
	}
	if len(histories) != 1 || histories[0].ClickID != clickID {
		t.Fatalf("histories = %+v, want exactly click %q", histories, clickID)
	}
}

func TestClicksByFTDAnchorFiltersByDimension(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	campaignA := idgen.New()
	campaignB := idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.CpaAccept, EventAt: day, OrganizationID: orgID, CampaignID: campaignA, ClickID: idgen.New(), Country: "US"},
		{Type: event.CpaAccept, EventAt: day, OrganizationID: orgID, CampaignID: campaignB, ClickID: idgen.New(), Country: "DE"},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	histories, err := store.ClicksByFTDAnchor(ctx, orgID, chstore.LTVFilter{
		From: day.Add(-time.Hour), To: day.Add(time.Hour), CampaignID: campaignA,
	})
	if err != nil {
		t.Fatalf("ClicksByFTDAnchor: %v", err)
	}
	if len(histories) != 1 || histories[0].CampaignID != campaignA {
		t.Fatalf("histories = %+v, want only campaignA", histories)
	}

	histories, err = store.ClicksByFTDAnchor(ctx, orgID, chstore.LTVFilter{
		From: day.Add(-time.Hour), To: day.Add(time.Hour), Country: "DE",
	})
	if err != nil {
		t.Fatalf("ClicksByFTDAnchor: %v", err)
	}
	if len(histories) != 1 || histories[0].Country != "DE" {
		t.Fatalf("histories = %+v, want only DE", histories)
	}
}

func TestClicksByFTDAnchorNoMatches(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)

	histories, err := store.ClicksByFTDAnchor(context.Background(), idgen.New(), chstore.LTVFilter{
		From: time.Now(), To: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ClicksByFTDAnchor: %v", err)
	}
	if len(histories) != 0 {
		t.Fatalf("histories = %d, want 0", len(histories))
	}
}

func TestLTVFilterBySourceAndOffer(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()
	orgID := idgen.New()
	srcA, srcB, offA, offB := idgen.New(), idgen.New(), idgen.New(), idgen.New()
	c1, c2, c3 := idgen.New(), idgen.New(), idgen.New()
	day := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	accept := func(click, src, off string) event.Event {
		return event.Event{Type: event.CpaAccept, EventAt: day, OrganizationID: orgID, ClickID: click, TrafficSourceID: src, OfferID: off, USDValue: 1, HasUSDValue: true}
	}
	if err := store.InsertBatch(ctx, []event.Event{accept(c1, srcA, offA), accept(c2, srcA, offB), accept(c3, srcB, offA)}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	month := chstore.LTVFilter{From: day.AddDate(0, 0, -9), To: day.AddDate(0, 0, 20)}
	for name, tc := range map[string]struct {
		filter func(*chstore.LTVFilter)
		want   int
	}{
		"none":          {func(*chstore.LTVFilter) {}, 3},
		"source":        {func(f *chstore.LTVFilter) { f.TrafficSourceID = srcA }, 2},
		"offer":         {func(f *chstore.LTVFilter) { f.OfferID = offA }, 2},
		"source+offer":  {func(f *chstore.LTVFilter) { f.TrafficSourceID, f.OfferID = srcB, offA }, 1},
		"no such offer": {func(f *chstore.LTVFilter) { f.OfferID = "nope" }, 0},
	} {
		f := month
		tc.filter(&f)
		histories, err := store.ClicksByFTDAnchor(ctx, orgID, f)
		if err != nil || len(histories) != tc.want {
			t.Fatalf("%s: %d histories, %v; want %d", name, len(histories), err, tc.want)
		}
		for _, h := range histories {
			if (f.TrafficSourceID != "" && h.TrafficSourceID != f.TrafficSourceID) || (f.OfferID != "" && h.OfferID != f.OfferID) {
				t.Fatalf("%s: history %+v does not match the filter", name, h)
			}
		}
	}
}

func TestBackfillLTVAttribution(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()
	orgID := idgen.New()
	name := "ltv_attrs_" + idgen.New()
	t.Cleanup(func() {
		_ = conn.Exec(chstore.SyncMutations(context.Background()), `ALTER TABLE chstore_backfills DELETE WHERE name = ?`, name)
	})
	at := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	old, kept, noOffer := idgen.New(), idgen.New(), idgen.New()
	if err := store.InsertBatch(ctx, []event.Event{
		// The click knows its offer and source; the conversion row, as
		// from before schema/018, does not.
		{Type: event.SourceClick, EventAt: at, OrganizationID: orgID, ClickID: old, OfferID: "off-click", TrafficSourceID: "src-click"},
		{Type: event.CpaAccept, EventAt: at.Add(time.Hour), OrganizationID: orgID, ClickID: old},
		// Already set: left alone.
		{Type: event.SourceClick, EventAt: at, OrganizationID: orgID, ClickID: kept, OfferID: "off-click", TrafficSourceID: "src-click"},
		{Type: event.CpaAccept, EventAt: at.Add(time.Hour), OrganizationID: orgID, ClickID: kept, OfferID: "off-then", TrafficSourceID: "src-then"},
		// A click without an offer stays empty.
		{Type: event.SourceClick, EventAt: at, OrganizationID: orgID, ClickID: noOffer},
		{Type: event.CpaAccept, EventAt: at.Add(time.Hour), OrganizationID: orgID, ClickID: noOffer},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	if err := chstore.RunLTVAttributionBackfill(ctx, conn, name); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	for click, want := range map[string][2]string{old: {"off-click", "src-click"}, kept: {"off-then", "src-then"}, noOffer: {"", ""}} {
		var offer, source string
		if err := conn.QueryRow(ctx, `SELECT offer_id, traffic_source_id FROM ltv_events WHERE organization_id = ? AND click_id = ?`, orgID, click).Scan(&offer, &source); err != nil {
			t.Fatalf("%s: %v", click, err)
		}
		if offer != want[0] || source != want[1] {
			t.Fatalf("click %s = %q/%q, want %q/%q", click, offer, source, want[0], want[1])
		}
	}
	var leftover uint64
	if err := conn.QueryRow(ctx, `SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = ?`, "backfill_attrs_"+name).Scan(&leftover); err != nil || leftover != 0 {
		t.Fatalf("scratch join table left behind: %d, %v", leftover, err)
	}
}

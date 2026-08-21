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
	store := chstore.NewEventStore(conn)
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
	store := chstore.NewEventStore(conn)
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
	store := chstore.NewEventStore(conn)
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
	store := chstore.NewEventStore(conn)

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

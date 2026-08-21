package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestListAndCountConversions(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	campaignID := idgen.New()
	networkID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	events := []event.Event{
		{Type: event.CpaHold, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), NetworkID: networkID},
		{Type: event.CpaAccept, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), NetworkID: networkID, Revenue: 25, Currency: "USD", USDValue: 25, HasUSDValue: true},
		{Type: event.CpaTrash, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), NetworkID: networkID},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	total, err := store.CountConversions(ctx, orgID, day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("CountConversions: %v", err)
	}
	if total != 3 {
		t.Fatalf("CountConversions = %d, want 3", total)
	}

	rows, err := store.ListConversions(ctx, orgID, day, day.Add(24*time.Hour), 2, 0)
	if err != nil {
		t.Fatalf("ListConversions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListConversions with limit 2 = %d rows, want 2", len(rows))
	}
	// newest first
	if !rows[0].EventAt.After(rows[1].EventAt) {
		t.Fatalf("ListConversions not newest-first: %+v", rows)
	}
	if rows[0].Type != event.CpaTrash {
		t.Fatalf("ListConversions[0].Type = %q, want CPA_TRASH (newest)", rows[0].Type)
	}

	page2, err := store.ListConversions(ctx, orgID, day, day.Add(24*time.Hour), 2, 2)
	if err != nil {
		t.Fatalf("ListConversions page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("ListConversions page 2 (offset 2, limit 2) = %d rows, want 1", len(page2))
	}
}

func TestConversionsByClickIDReturnsFullStatusHistory(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	campaignID := idgen.New()
	clickID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	events := []event.Event{
		{Type: event.CpaHold, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID},
		{Type: event.CpaAccept, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID, Revenue: 30, Currency: "USD", USDValue: 30, HasUSDValue: true},
		{Type: event.CpaRedep, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID, Revenue: 10, Currency: "USD", USDValue: 10, HasUSDValue: true},
		// a different click_id must not leak into this click_id's history
		{Type: event.CpaHold, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	rows, err := store.ConversionsByClickID(ctx, orgID, clickID)
	if err != nil {
		t.Fatalf("ConversionsByClickID: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("ConversionsByClickID = %d rows, want 3 (HOLD, ACCEPT, REDEP)", len(rows))
	}
	wantOrder := []event.Type{event.CpaHold, event.CpaAccept, event.CpaRedep}
	for i, w := range wantOrder {
		if rows[i].Type != w {
			t.Fatalf("ConversionsByClickID[%d].Type = %q, want %q (oldest first)", i, rows[i].Type, w)
		}
	}
}

func TestFunnelByClickIDMergesClickAndTrackingEvents(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	campaignID := idgen.New()
	clickID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	events := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID},
		{Type: event.LandView, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID},
		{Type: event.PwaInstall, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	funnel, err := store.FunnelByClickID(ctx, orgID, clickID)
	if err != nil {
		t.Fatalf("FunnelByClickID: %v", err)
	}
	if len(funnel) != 3 {
		t.Fatalf("FunnelByClickID = %d events, want 3 (SOURCE_CLICK + LAND_VIEW + PWA_INSTALL)", len(funnel))
	}
	for _, f := range funnel {
		if f.CampaignID != campaignID {
			t.Fatalf("FunnelByClickID event %+v has wrong campaign id, want %s", f, campaignID)
		}
	}
}

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
	store := chstore.NewRawEventStore(conn)
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

	total, err := store.CountConversions(ctx, orgID, day, day.Add(24*time.Hour), "", nil)
	if err != nil {
		t.Fatalf("CountConversions: %v", err)
	}
	if total != 3 {
		t.Fatalf("CountConversions = %d, want 3", total)
	}

	rows, err := store.ListConversions(ctx, orgID, day, day.Add(24*time.Hour), "", 2, 0, nil)
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

	page2, err := store.ListConversions(ctx, orgID, day, day.Add(24*time.Hour), "", 2, 2, nil)
	if err != nil {
		t.Fatalf("ListConversions page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("ListConversions page 2 (offset 2, limit 2) = %d rows, want 1", len(page2))
	}
}

func TestConversionsByClickIDReturnsFullStatusHistory(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
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
	store := chstore.NewRawEventStore(conn)
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

func TestStatusHistoryAndOffersByClickIDsAreOrgScoped(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()

	orgID, otherOrg := idgen.New(), idgen.New()
	campaignID, networkID := idgen.New(), idgen.New()
	click, noOfferClick := idgen.New(), idgen.New()
	day := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)

	var subs event.Subs
	subs[0], subs[9] = "creative-7", "tail"
	events := []event.Event{
		{Type: event.SourceClick, EventAt: day, OrganizationID: orgID, CampaignID: campaignID, ClickID: click, OfferID: "offer-1"},
		{Type: event.SourceClick, EventAt: day, OrganizationID: orgID, CampaignID: campaignID, ClickID: noOfferClick},
		{Type: event.CpaHold, EventAt: day.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, NetworkID: networkID, FlowID: "flow-1", Subs: subs, NetworkTxnID: "txn-1"},
		{Type: event.CpaAccept, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: click, NetworkID: networkID},
		// same click id in another organization: must never appear for orgID
		{Type: event.SourceClick, EventAt: day, OrganizationID: otherOrg, CampaignID: campaignID, ClickID: click, OfferID: "offer-evil"},
		{Type: event.CpaTrash, EventAt: day.Add(3 * time.Hour), OrganizationID: otherOrg, CampaignID: campaignID, ClickID: click, NetworkID: networkID},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	steps, err := store.StatusHistoryByClickIDs(ctx, orgID, []string{click, noOfferClick})
	if err != nil {
		t.Fatalf("StatusHistoryByClickIDs: %v", err)
	}
	if len(steps) != 2 || steps[0].Type != event.CpaHold || steps[1].Type != event.CpaAccept {
		t.Fatalf("steps = %+v, want this org's HOLD then ACCEPT only", steps)
	}

	offers, err := store.OffersByClickIDs(ctx, orgID, []string{click, noOfferClick})
	if err != nil {
		t.Fatalf("OffersByClickIDs: %v", err)
	}
	if offers[click] != "offer-1" {
		t.Fatalf("offers[click] = %q, want offer-1 (not the other org's)", offers[click])
	}
	if _, ok := offers[noOfferClick]; ok {
		t.Fatalf("offers has an entry for a click without an offer: %+v", offers)
	}

	rows, err := store.ListConversions(ctx, orgID, day, day.Add(24*time.Hour), "", 10, 0, nil)
	if err != nil {
		t.Fatalf("ListConversions: %v", err)
	}
	var hold *chstore.ConversionEvent
	for i := range rows {
		if rows[i].Type == event.CpaHold {
			hold = &rows[i]
		}
	}
	if hold == nil || hold.FlowID != "flow-1" || hold.NetworkTxnID != "txn-1" || hold.Subs[0] != "creative-7" || hold.Subs[9] != "tail" {
		t.Fatalf("hold row = %+v, want flow/txn/subs read back", hold)
	}

	if steps, err := store.StatusHistoryByClickIDs(ctx, orgID, nil); err != nil || steps != nil {
		t.Fatalf("empty click id list = %v, %v; want nil, nil", steps, err)
	}
}

func TestNewConversionsByOfferCountsFirstSeenPerClickAndIsOrgScoped(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	ctx := context.Background()

	orgID, otherOrg := idgen.New(), idgen.New()
	campaignID, networkID, offerID := idgen.New(), idgen.New(), idgen.New()
	since := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	hold := func(org, click string, at time.Time) event.Event {
		return event.Event{Type: event.CpaHold, EventAt: at, OrganizationID: org, CampaignID: campaignID, ClickID: click, NetworkID: networkID, OfferID: offerID}
	}
	accept := func(org, click string, at time.Time) event.Event {
		e := hold(org, click, at)
		e.Type = event.CpaAccept
		return e
	}
	c1, c2, c3, c4 := idgen.New(), idgen.New(), idgen.New(), idgen.New()
	events := []event.Event{
		// c1: HOLD then ACCEPT the same day -> one conversion
		hold(orgID, c1, since.Add(time.Hour)), accept(orgID, c1, since.Add(2*time.Hour)),
		// c2: HOLD yesterday, ACCEPT today -> not new today
		hold(orgID, c2, since.Add(-3*time.Hour)), accept(orgID, c2, since.Add(time.Hour)),
		// c3: direct ACCEPT today -> one conversion
		accept(orgID, c3, since.Add(3*time.Hour)),
		// c4: a DECLINE alone is not a conversion
		{Type: event.CpaDecline, EventAt: since.Add(time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: c4, NetworkID: networkID, OfferID: offerID},
		// the same offer id in another organization must never be counted
		hold(otherOrg, idgen.New(), since.Add(time.Hour)), hold(otherOrg, idgen.New(), since.Add(time.Hour)),
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	got, err := store.NewConversionsByOffer(ctx, []chstore.OfferConversionTarget{
		{OrganizationID: orgID, OfferID: offerID, Since: since},
	})
	if err != nil {
		t.Fatalf("NewConversionsByOffer: %v", err)
	}
	if n := got[orgID+"/"+offerID]; n != 2 {
		t.Fatalf("count = %d, want 2 (c1 and c3; got map %v)", n, got)
	}
	if _, leaked := got[otherOrg+"/"+offerID]; leaked {
		t.Fatalf("other organization's conversions leaked: %v", got)
	}
}

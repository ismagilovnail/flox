package chstore_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestClickResolverByClickID(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	clickID := idgen.New()
	campaignID := idgen.New()
	occurredAt := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)

	err := store.InsertBatch(ctx, []event.Event{{
		Type: event.SourceClick, EventAt: occurredAt, OrganizationID: orgID, CampaignID: campaignID,
		ClickID: clickID, Country: "US", Device: "mobile", ExternalClickID: "fb.123",
		UTMSource: "facebook", Subs: event.Subs{"a", "b"}, LandingID: "landing-1", OfferID: "offer-1", PwaID: "pwa-1", TrafficSourceID: "source-1",
		PostlandingID: "postlanding-1",
	}})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	click, err := resolver.ByClickID(ctx, orgID, clickID)
	if err != nil {
		t.Fatalf("ByClickID: %v", err)
	}
	if click.ClickID != clickID || click.CampaignID != campaignID || click.Country != "US" || click.Device != "mobile" {
		t.Fatalf("click = %+v, want matching the inserted row", click)
	}
	if click.ExternalClickID != "fb.123" || click.UTMSource != "facebook" || click.Subs[0] != "a" || click.Subs[1] != "b" || click.LandingID != "landing-1" || click.OfferID != "offer-1" || click.PwaID != "pwa-1" || click.TrafficSourceID != "source-1" || click.PostlandingID != "postlanding-1" {
		t.Fatalf("click pass-through fields wrong: %+v", click)
	}
	if !click.OccurredAt.Equal(occurredAt) {
		t.Fatalf("OccurredAt = %v, want %v", click.OccurredAt, occurredAt)
	}
}

func TestClickResolverByClickIDNotFound(t *testing.T) {
	conn := mustConn(t)
	resolver := chstore.NewClickResolver(conn)

	_, err := resolver.ByClickID(context.Background(), idgen.New(), idgen.New())
	if !errors.Is(err, attribution.ErrClickNotFound) {
		t.Fatalf("err = %v, want ErrClickNotFound", err)
	}
}

// TestClickResolverExcludesFilteredClicks: a SOURCE_FILTER row never
// reached a destination, so it must never resolve as an attributable click
// — see attribution.go's comment on clickColumns' type filter.
func TestClickResolverExcludesFilteredClicks(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	clickID := idgen.New()
	if err := store.InsertBatch(ctx, []event.Event{{
		Type: event.SourceFilter, EventAt: time.Now().UTC(), OrganizationID: orgID, ClickID: clickID, FilterReason: "bot",
	}}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	_, err := resolver.ByClickID(ctx, orgID, clickID)
	if !errors.Is(err, attribution.ErrClickNotFound) {
		t.Fatalf("err = %v, want ErrClickNotFound for a filtered click", err)
	}
}

// TestClickResolverByClickIDPicksEarliest: stickyFlowKeepClickId reuses one
// click_id across a returning visitor's whole journey (§39-STICKY) — the
// resolver must resolve to the ORIGINAL click, not the most recent one.
func TestClickResolverByClickIDPicksEarliest(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	clickID := idgen.New()
	first := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	second := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)

	err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: second, OrganizationID: orgID, ClickID: clickID, CampaignID: "second-visit-campaign"},
		{Type: event.SourceClick, EventAt: first, OrganizationID: orgID, ClickID: clickID, CampaignID: "first-visit-campaign"},
	})
	if err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	click, err := resolver.ByClickID(ctx, orgID, clickID)
	if err != nil {
		t.Fatalf("ByClickID: %v", err)
	}
	if click.CampaignID != "first-visit-campaign" {
		t.Fatalf("resolved campaign = %q, want the earliest visit's campaign", click.CampaignID)
	}
}

func TestClickResolverByExternalClickID(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgID := idgen.New()
	externalID := "fbclid." + idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: time.Now().UTC(), OrganizationID: orgID, ClickID: idgen.New(), ExternalClickID: externalID},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	clicks, err := resolver.ByExternalClickID(ctx, orgID, externalID)
	if err != nil {
		t.Fatalf("ByExternalClickID: %v", err)
	}
	if len(clicks) != 1 {
		t.Fatalf("clicks = %d, want exactly 1", len(clicks))
	}

	// A second click sharing the same external_click_id makes it ambiguous
	// at the attribution layer — this resolver's job is only to report
	// every match, not to pick one.
	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: time.Now().UTC(), OrganizationID: orgID, ClickID: idgen.New(), ExternalClickID: externalID},
	}); err != nil {
		t.Fatalf("InsertBatch (second): %v", err)
	}
	clicks, err = resolver.ByExternalClickID(ctx, orgID, externalID)
	if err != nil {
		t.Fatalf("ByExternalClickID (second): %v", err)
	}
	if len(clicks) != 2 {
		t.Fatalf("clicks = %d, want exactly 2", len(clicks))
	}
}

// Audit DATA-7: external_click_id holds only what the link sent as
// external_click_id; fbclid and ttclid are stored once, in their own
// columns. The source's click id still reads as before — external, else
// fbclid, else ttclid — for new rows and for older rows that carry the
// copy, and attribution's lookup by it still goes through a bloom filter.
func TestSourceClickIDFallsBackToFbclidAndTtclid(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()
	orgID := idgen.New()
	fbOnly, ttOnly, both, legacy := idgen.New(), idgen.New(), idgen.New(), idgen.New()
	fb, tt, ext, fbOfBoth, legacyFb := "fb."+idgen.New(), "tt."+idgen.New(), "ext."+idgen.New(), "fb."+idgen.New(), "fb."+idgen.New()
	now := time.Now().UTC()
	click := func(id string) event.Event {
		return event.Event{Type: event.SourceClick, EventAt: now, OrganizationID: orgID, ClickID: id}
	}
	evs := []event.Event{click(fbOnly), click(ttOnly), click(both), click(legacy)}
	evs[0].FBClickID = fb
	evs[1].TTClickID = tt
	evs[2].ExternalClickID, evs[2].FBClickID = ext, fbOfBoth
	evs[3].ExternalClickID, evs[3].FBClickID = legacyFb, legacyFb // as stored before DATA-7
	if err := store.InsertBatch(ctx, evs); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	for _, c := range []struct {
		lookup, want string
		found        bool
	}{
		{fb, fbOnly, true},
		{tt, ttOnly, true},
		{ext, both, true},
		{fbOfBoth, "", false}, // an explicit external_click_id wins, as before
		{legacyFb, legacy, true},
	} {
		clicks, err := resolver.ByExternalClickID(ctx, orgID, c.lookup)
		if err != nil {
			t.Fatalf("ByExternalClickID(%s): %v", c.lookup, err)
		}
		if !c.found {
			if len(clicks) != 0 {
				t.Errorf("ByExternalClickID(%s) = %d clicks, want none", c.lookup, len(clicks))
			}
			continue
		}
		if len(clicks) != 1 || clicks[0].ClickID != c.want || clicks[0].ExternalClickID != c.lookup {
			t.Errorf("ByExternalClickID(%s) = %+v, want click %s reading external id %s", c.lookup, clicks, c.want, c.lookup)
		}
		got, err := resolver.ByClickID(ctx, orgID, c.want)
		if err != nil || got.ExternalClickID != c.lookup {
			t.Errorf("ByClickID(%s).ExternalClickID = %q (%v), want %s", c.want, got.ExternalClickID, err, c.lookup)
		}
	}

	rows, err := conn.Query(ctx, `EXPLAIN indexes = 1 SELECT click_id FROM click_events
		WHERE organization_id = ? AND `+chstore.SourceClickIDExpr+` = ?`, orgID, fb)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if !strings.Contains(plan.String(), "idx_source_click_id") {
		t.Errorf("the lookup by source click id does not use idx_source_click_id:\n%s", plan.String())
	}
}

// Audit UNI-2: the network's click id is stored on the click and read
// back with it — by the lookup offline conversions will use, and in the
// click log.
func TestAdClickIDIsStoredWithTheClick(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()
	orgID := idgen.New()
	clickID := idgen.New()
	now := time.Now().UTC()
	if err := store.InsertBatch(ctx, []event.Event{{
		Type: event.SourceClick, EventAt: now, OrganizationID: orgID, ClickID: clickID,
		AdClickParam: "yclid", AdClickID: "7364217983742390271",
	}}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	got, err := resolver.ByClickID(ctx, orgID, clickID)
	if err != nil || got.AdClickParam != "yclid" || got.AdClickID != "7364217983742390271" {
		t.Fatalf("ByClickID = %q=%q (%v), want yclid=7364217983742390271", got.AdClickParam, got.AdClickID, err)
	}
	rows, err := store.ListClickLog(ctx, orgID, now.Add(-time.Minute), now.Add(time.Minute), clickID, 10, 0, nil)
	if err != nil || len(rows) != 1 || rows[0].AdClickParam != "yclid" || rows[0].AdClickID != "7364217983742390271" {
		t.Fatalf("ListClickLog = %+v (%v), want the yclid", rows, err)
	}
}

func TestClickResolverByExternalClickIDNoMatch(t *testing.T) {
	conn := mustConn(t)
	resolver := chstore.NewClickResolver(conn)

	clicks, err := resolver.ByExternalClickID(context.Background(), idgen.New(), "nonexistent")
	if err != nil {
		t.Fatalf("ByExternalClickID: %v", err)
	}
	if len(clicks) != 0 {
		t.Fatalf("clicks = %d, want 0", len(clicks))
	}
}

func TestClickResolverTenantIsolation(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewRawEventStore(conn)
	resolver := chstore.NewClickResolver(conn)
	ctx := context.Background()

	orgA := idgen.New()
	orgB := idgen.New()
	clickID := idgen.New()

	if err := store.InsertBatch(ctx, []event.Event{
		{Type: event.SourceClick, EventAt: time.Now().UTC(), OrganizationID: orgA, ClickID: clickID},
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	_, err := resolver.ByClickID(ctx, orgB, clickID)
	if !errors.Is(err, attribution.ErrClickNotFound) {
		t.Fatalf("org B resolving org A's click: err = %v, want ErrClickNotFound", err)
	}
}

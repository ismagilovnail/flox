package chstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Audit 2026-10-06, D2: a retried batch adds nothing — whether the first
// attempt fully landed (insert timed out after commit, reaped after a crash)
// or only some of its tables did (clicks in, conversions failed) — in the
// raw tables and in the materialized views built from them.
func TestRetriedBatchIsNotDuplicated(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	orgID, campaignID := idgen.New(), idgen.New()
	at := time.Now().UTC().Truncate(time.Second)

	click := event.Event{Type: event.SourceClick, OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), EventAt: at, Country: "DE"}
	conv := event.Event{Type: event.CpaAccept, OrganizationID: orgID, CampaignID: campaignID, ClickID: click.ClickID, EventAt: at,
		Revenue: 10, Currency: "USD", USDValue: 10, HasUSDValue: true}

	batch := chstore.WithBatchID(ctx, idgen.New())
	// First attempt: only the click table landed.
	if err := store.InsertBatch(batch, []event.Event{click}); err != nil {
		t.Fatalf("partial first attempt: %v", err)
	}
	// Retry of the whole batch, twice.
	for i := 0; i < 2; i++ {
		if err := store.InsertBatch(batch, []event.Event{click, conv}); err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}
	}
	// A different batch with the same rows is new data, not a retry.
	if err := store.InsertBatch(chstore.WithBatchID(ctx, idgen.New()), []event.Event{click}); err != nil {
		t.Fatalf("second batch: %v", err)
	}

	count := func(query string) uint64 {
		var n uint64
		if err := conn.QueryRow(ctx, query, orgID).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}
	for query, want := range map[string]uint64{
		`SELECT count() FROM click_events WHERE organization_id = ?`:                                        2,
		`SELECT toUInt64(sum(event_count)) FROM click_events_daily_campaign WHERE organization_id = ?`:      2,
		`SELECT toUInt64(sum(event_count)) FROM click_events_daily_geo WHERE organization_id = ?`:           2,
		`SELECT count() FROM conversion_events WHERE organization_id = ?`:                                   1,
		`SELECT toUInt64(sum(event_count)) FROM conversion_events_daily_campaign WHERE organization_id = ?`: 1,
		`SELECT toUInt64(sum(revenue_usd)) FROM conversion_events_daily_campaign WHERE organization_id = ?`: 10,
		`SELECT count() FROM ltv_events WHERE organization_id = ?`:                                          1,
	} {
		if got := count(query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
}

func TestRetriedPostbackAttemptBatchIsNotDuplicated(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := chstore.WithBatchID(context.Background(), idgen.New())
	orgID := idgen.New()
	attempts := []chstore.PostbackAttempt{{OrganizationID: orgID, EventAt: time.Now().UTC(), Direction: "incoming", ClickID: idgen.New(), Result: "success"}}

	for i := 0; i < 3; i++ {
		if err := store.InsertPostbackAttempts(ctx, attempts); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	var n uint64
	if err := conn.QueryRow(context.Background(), `SELECT count() FROM postback_events WHERE organization_id = ?`, orgID).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n != 1 {
		t.Fatalf("postback_events rows = %d, want 1", n)
	}
}

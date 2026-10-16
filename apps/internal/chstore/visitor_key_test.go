package chstore_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/visitorkey"
)

// Audit 2026-10-15, GDPR-1: raw IP and user agent are wiped after
// RETENTION_IP_DAYS; unique clicks count the visitor's pseudonym, so an
// old period reads the same before and after the wipe. Clicks from before
// the pseudonym get it from the backfill, computed exactly as the tracker
// does.
func TestUniqueClicksSurviveTheIPWipe(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()
	key := bytes.Repeat([]byte{3}, 32)
	keyOf := func(ip, ua string) string { return visitorkey.Of(key, ip, ua) }
	orgID, campaignID := idgen.New(), idgen.New()
	day := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)

	click := func(ip, ua string, withKey bool) event.Event {
		e := event.Event{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID,
			CampaignID: campaignID, ClickID: idgen.New(), IP: ip, UserAgent: ua}
		if withKey {
			e.VisitorKey = keyOf(ip, ua)
		}
		return e
	}
	// Visitor A three times (twice from the tracker with the pseudonym,
	// once from before it), visitor B once from before it, and a click
	// with neither IP nor user agent: 3 visitors.
	if err := store.InsertBatch(ctx, []event.Event{
		click("203.0.113.1", "UA-a", true),
		click("203.0.113.1", "UA-a", true),
		click("203.0.113.1", "UA-a", false),
		click("203.0.113.2", "UA-b", false),
		click("", "", false),
	}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	unique := func(stage string) uint64 {
		t.Helper()
		aggs, err := store.ReportClicks(ctx, chstore.ReportQuery{OrganizationID: orgID, From: day, To: day.Add(24 * time.Hour)})
		if err != nil || len(aggs) != 1 || aggs[0].Clicks != 5 {
			t.Fatalf("%s: ReportClicks = %+v, %v; want one row of 5 clicks", stage, aggs, err)
		}
		return aggs[0].UniqueClicks
	}

	name := "visitor_keys_" + idgen.New()
	t.Cleanup(func() {
		_ = conn.Exec(chstore.SyncMutations(context.Background()), `ALTER TABLE chstore_backfills DELETE WHERE name = ?`, name)
	})
	if err := chstore.RunVisitorKeyBackfill(ctx, conn, name, keyOf, orgID); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	// The backfilled pseudonym is the tracker's: A's older click joins A.
	if got := unique("after the backfill"); got != 3 {
		t.Fatalf("unique clicks after the backfill = %d, want 3", got)
	}
	var missing uint64
	if err := conn.QueryRow(ctx, `SELECT count() FROM click_events WHERE organization_id = ? AND visitor_key = '' AND ip != ''`, orgID).Scan(&missing); err != nil || missing != 0 {
		t.Fatalf("clicks with an IP but no pseudonym = %d, %v; want 0", missing, err)
	}

	// What the TTL does to an expired row: ip and user_agent become ''.
	if err := conn.Exec(chstore.SyncMutations(ctx), `ALTER TABLE click_events UPDATE ip = '', user_agent = '' WHERE organization_id = ?`, orgID); err != nil {
		t.Fatal(err)
	}
	if got := unique("after the wipe"); got != 3 {
		t.Fatalf("unique clicks after the wipe = %d, want 3 — the report must not change", got)
	}
}

// The worker sets the TTL on ip and user_agent in every table that holds
// them; only a change alters a table. Ends on 90 days, the default the dev
// worker applies.
func TestSetPersonalDataTTL(t *testing.T) {
	conn := mustConn(t)
	ctx := context.Background()
	t.Cleanup(func() { _, _ = chstore.SetPersonalDataTTL(context.Background(), conn, 90) })
	for _, step := range []struct {
		days        int
		wantChanged int
	}{
		// ip + user_agent in three tables, referrer in two.
		{0, 8}, {0, 0}, {90, 8}, {90, 0}, {9, 8}, {90, 8},
	} {
		changed, err := chstore.SetPersonalDataTTL(ctx, conn, step.days)
		if err != nil {
			t.Fatalf("days %d: %v", step.days, err)
		}
		if len(changed) != step.wantChanged {
			t.Fatalf("days %d: changed %v, want %d columns", step.days, changed, step.wantChanged)
		}
	}
}

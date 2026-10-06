package chstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/attribution"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

// Audit 2026-10-06, CH1: a click recorded before click_lookup existed is
// only in click_events. Until the backfill is recorded the resolver falls
// back to scanning there; once it is, a click_lookup miss is final — no
// scan of the organization's history.
func TestByClickIDFallsBackOnlyUntilBackfilled(t *testing.T) {
	conn := mustConn(t)
	ctx := context.Background()
	orgID, clickID := idgen.New(), idgen.New()
	if err := chstore.NewEventStore(conn).InsertBatch(ctx, []event.Event{{
		Type: event.SourceClick, EventAt: time.Now().UTC(), OrganizationID: orgID, ClickID: clickID, CampaignID: "camp",
	}}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	// Make it a pre-click_lookup click: present in click_events only.
	if err := conn.Exec(chstore.SyncMutations(ctx), `ALTER TABLE click_lookup DELETE WHERE organization_id = ?`, orgID); err != nil {
		t.Fatalf("removing the lookup row: %v", err)
	}

	pending := chstore.NewClickResolverWithBackfill(conn, "not_yet_"+idgen.New())
	click, err := pending.ByClickID(ctx, orgID, clickID)
	if err != nil || click.CampaignID != "camp" {
		t.Fatalf("before the backfill: %+v, %v — want the click found in click_events", click, err)
	}

	done := "done_" + idgen.New()
	if err := chstore.RunBackfill(ctx, conn, done, `SELECT 1`); err != nil {
		t.Fatalf("recording a finished backfill: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Exec(chstore.SyncMutations(context.Background()), `ALTER TABLE chstore_backfills DELETE WHERE name = ?`, done)
	})
	if _, err := chstore.NewClickResolverWithBackfill(conn, done).ByClickID(ctx, orgID, clickID); !errors.Is(err, attribution.ErrClickNotFound) {
		t.Fatalf("after the backfill: err = %v, want ErrClickNotFound without scanning click_events", err)
	}
}

// A backfill runs once: a second Backfill with the same name is a no-op.
func TestBackfillRunsOnce(t *testing.T) {
	conn := mustConn(t)
	ctx := context.Background()
	orgID := idgen.New()
	name := "once_" + idgen.New()
	t.Cleanup(func() {
		_ = conn.Exec(chstore.SyncMutations(context.Background()), `ALTER TABLE chstore_backfills DELETE WHERE name = ?`, name)
	})

	copyOne := `INSERT INTO click_lookup (organization_id, click_id, event_at) SELECT '` + orgID + `', generateUUIDv4(), now64(3)`
	for i := 0; i < 3; i++ {
		if err := chstore.RunBackfill(ctx, conn, name, copyOne); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	var rows uint64
	if err := conn.QueryRow(ctx, `SELECT count() FROM click_lookup WHERE organization_id = ?`, orgID).Scan(&rows); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if rows != 1 {
		t.Fatalf("backfill rows = %d after 3 runs, want 1 (it must run once)", rows)
	}
}

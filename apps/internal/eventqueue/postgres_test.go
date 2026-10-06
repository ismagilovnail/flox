package eventqueue_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/eventbuf"
	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/testpg"
)

func TestEnqueueClaimAndDelete(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	table := testpg.PrivateTable(t, pool, "event_queue")
	q := eventqueue.NewPostgresQueueOnTable(pool, table)
	clickID := idgen.New()
	events := []event.Event{
		{Type: event.SourceClick, OrganizationID: orgID, ClickID: clickID, EventAt: time.Now().UTC()},
		{Type: event.LandView, OrganizationID: orgID, ClickID: clickID, EventAt: time.Now().UTC()},
	}
	if err := q.EnqueueBatch(ctx, events); err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}

	claimed, err := q.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	// Filter to just this test's rows since the table has no per-test
	// isolation (it's a transit queue, not tenant-scoped reads).
	var mine []eventqueue.QueuedEvent
	for _, c := range claimed {
		if c.Event.ClickID == clickID {
			mine = append(mine, c)
		}
	}
	if len(mine) != 2 {
		t.Fatalf("claimed = %d matching events, want 2 (got %d total claimed)", len(mine), len(claimed))
	}

	types := map[event.Type]bool{}
	for _, c := range mine {
		types[c.Event.Type] = true
		if c.Event.OrganizationID != orgID {
			t.Fatalf("claimed event organization_id = %q, want %q", c.Event.OrganizationID, orgID)
		}
	}
	if !types[event.SourceClick] || !types[event.LandView] {
		t.Fatalf("claimed types = %v, want SOURCE_CLICK and LAND_VIEW", types)
	}

	// A claimed (now 'processing') row is not due again.
	claimedAgain, err := q.ClaimDue(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimDue (second): %v", err)
	}
	for _, c := range claimedAgain {
		if c.Event.ClickID == clickID {
			t.Fatalf("claimed a 'processing' event a second time: %+v", c)
		}
	}

	ids := make([]string, len(mine))
	for i, c := range mine {
		ids[i] = c.ID
	}
	if err := q.Delete(ctx, ids); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE payload->>'clickId' = $1`, clickID).Scan(&count); err != nil {
		t.Fatalf("counting remaining rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("rows remaining after Delete = %d, want 0", count)
	}
}

func TestRequeuePutsRowsBackAsDueAtNextAttemptAt(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	table := testpg.PrivateTable(t, pool, "event_queue")
	q := eventqueue.NewPostgresQueueOnTable(pool, table)
	clickID := idgen.New()

	if err := q.EnqueueBatch(ctx, []event.Event{{Type: event.SourceClick, OrganizationID: orgID, ClickID: clickID}}); err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}
	claimed, err := q.ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	var id string
	for _, c := range claimed {
		if c.Event.ClickID == clickID {
			id = c.ID
		}
	}
	if id == "" {
		t.Fatal("did not claim the enqueued event")
	}

	future := time.Now().UTC().Add(time.Hour)
	if err := q.Requeue(ctx, []string{id}, future); err != nil {
		t.Fatalf("Requeue: %v", err)
	}

	notYetDue, err := q.ClaimDue(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	for _, c := range notYetDue {
		if c.ID == id {
			t.Fatal("claimed an event requeued an hour into the future")
		}
	}

	if err := q.Requeue(ctx, []string{id}, time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatalf("Requeue (backdated): %v", err)
	}
	dueNow, err := q.ClaimDue(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	found := false
	for _, c := range dueNow {
		if c.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("requeued event with a past next_attempt_at was not claimed")
	}
	_ = q.Delete(ctx, []string{id})
}

func TestDepthReflectsQueuedAndProcessingNotDeleted(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	table := testpg.PrivateTable(t, pool, "event_queue")
	q := eventqueue.NewPostgresQueueOnTable(pool, table)

	before, err := q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth (before): %v", err)
	}

	clickID := idgen.New()
	events := []event.Event{
		{Type: event.SourceClick, OrganizationID: orgID, ClickID: clickID, EventAt: time.Now().UTC()},
		{Type: event.LandView, OrganizationID: orgID, ClickID: clickID, EventAt: time.Now().UTC()},
		{Type: event.PwaView, OrganizationID: orgID, ClickID: clickID, EventAt: time.Now().UTC()},
	}
	if err := q.EnqueueBatch(ctx, events); err != nil {
		t.Fatalf("EnqueueBatch: %v", err)
	}

	afterEnqueue, err := q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth (after enqueue): %v", err)
	}
	if afterEnqueue != before+3 {
		t.Fatalf("Depth after enqueueing 3 = %d, want %d (before=%d)", afterEnqueue, before+3, before)
	}

	// Claiming moves rows to 'processing', not out of the queue — Depth
	// must still count them (a slow ClickHouse insert shouldn't make the
	// backlog gauge dip to zero mid-flush).
	claimed, err := q.ClaimDue(ctx, 100)
	if err != nil {
		t.Fatalf("ClaimDue: %v", err)
	}
	var ids []string
	for _, c := range claimed {
		if c.Event.ClickID == clickID {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("claimed %d matching events, want 3", len(ids))
	}

	afterClaim, err := q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth (after claim): %v", err)
	}
	if afterClaim != afterEnqueue {
		t.Fatalf("Depth after claiming (still undeleted) = %d, want unchanged at %d", afterClaim, afterEnqueue)
	}

	if err := q.Delete(ctx, ids); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	afterDelete, err := q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth (after delete): %v", err)
	}
	if afterDelete != before {
		t.Fatalf("Depth after deleting = %d, want back to %d", afterDelete, before)
	}
}

func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	id := idgen.New()
	_, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1, $2)`, id, "Test Org "+id)
	if err != nil {
		t.Fatalf("seeding organization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, id)
	})
	return id
}

// A batch mixing a live organization with a deleted one must still persist the
// live organization's events: the orphan's foreign-key violation may cost only
// its own events, not everyone else's in the same batch.
func TestEnqueueBatchOrphanOrganizationDoesNotDropTheRest(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)

	q := eventqueue.NewPostgresQueue(pool)
	liveClick, orphanClick := idgen.New(), idgen.New()
	err := q.EnqueueBatch(ctx, []event.Event{
		{Type: event.SourceClick, OrganizationID: idgen.New(), ClickID: orphanClick, EventAt: time.Now().UTC()}, // no such organization
		{Type: event.SourceClick, OrganizationID: orgID, ClickID: liveClick, EventAt: time.Now().UTC()},
	})
	if err == nil {
		t.Fatal("EnqueueBatch returned nil, want the orphan organization's error reported")
	}
	// Audit 2026-10-06, RL2: the orphan is refused for good and the live
	// event is written, so nothing is handed back to eventbuf for a retry —
	// retrying would duplicate the live event and loop forever on the orphan.
	var se *eventbuf.SinkError
	if !errors.As(err, &se) || se.Dropped != 1 || len(se.Retry) != 0 {
		t.Fatalf("err = %#v, want a SinkError dropping 1 event and retrying none", err)
	}

	// The SinkError above already says the live organization's event was
	// written (nothing to retry, only the orphan dropped). Its row is not
	// looked up: the dev-stack worker polls this same table and may have
	// flushed and deleted it already — this test used to claim rows itself
	// and flaked whenever it lost that race. The orphan, though, can never
	// appear, so its absence is checked directly.
	var orphanRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_queue WHERE payload->>'clickId' = $1`, orphanClick).Scan(&orphanRows); err != nil {
		t.Fatalf("counting orphan rows: %v", err)
	}
	if orphanRows != 0 {
		t.Fatalf("orphan organization's event was queued (%d rows), want none", orphanRows)
	}
}

// A transient failure (here: the context is gone) wrote nothing and is a
// plain error, so eventbuf retries the whole batch.
func TestEnqueueBatchTransientFailureIsRetryable(t *testing.T) {
	pool := mustPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	orgID := seedOrg(t, context.Background(), pool)
	cancel()

	err := eventqueue.NewPostgresQueue(pool).EnqueueBatch(ctx, []event.Event{
		{Type: event.SourceClick, OrganizationID: orgID, ClickID: idgen.New(), EventAt: time.Now().UTC()},
	})
	var se *eventbuf.SinkError
	if err == nil || errors.As(err, &se) {
		t.Fatalf("err = %#v, want a plain (retry everything) error", err)
	}
}

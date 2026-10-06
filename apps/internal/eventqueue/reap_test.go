package eventqueue_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/eventqueue"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func TestReapStuckReturnsOnlyAbandonedClaims(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	q := eventqueue.NewPostgresQueue(pool)

	// Rows are written straight into 'processing': a 'queued' row could be
	// claimed by a dev-stack worker polling the same table mid-test.
	stuck, fresh := idgen.New(), idgen.New()
	for clickID, age := range map[string]string{stuck: "10 minutes", fresh: "1 second"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO event_queue (id, organization_id, type, payload, status, claimed_at)
			VALUES ($1, $2, 'SOURCE_CLICK', jsonb_build_object('clickId', $3::text), 'processing', now() - $4::interval)`,
			idgen.New(), orgID, clickID, age,
		); err != nil {
			t.Fatalf("seeding claimed row: %v", err)
		}
	}

	if _, err := q.ReapStuck(ctx, 5*time.Minute); err != nil {
		t.Fatalf("ReapStuck: %v", err)
	}

	status := func(clickID string) (s string, claimed bool) {
		var claimedAt *time.Time
		if err := pool.QueryRow(ctx,
			`SELECT status, claimed_at FROM event_queue WHERE payload->>'clickId' = $1`, clickID,
		).Scan(&s, &claimedAt); err != nil {
			t.Fatalf("reading row: %v", err)
		}
		return s, claimedAt != nil
	}
	if s, c := status(stuck); s != "queued" || c {
		t.Fatalf("abandoned claim: status=%s claimed=%v, want queued/false", s, c)
	}
	if s, c := status(fresh); s != "processing" || !c {
		t.Fatalf("fresh claim: status=%s claimed=%v, want processing/true (must not be reaped)", s, c)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM event_queue WHERE payload->>'clickId' = ANY($1)`, []string{stuck, fresh}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}

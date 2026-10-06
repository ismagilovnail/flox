package postbacklog_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/postbacklog"
)

func TestPostgresQueueReapStuck(t *testing.T) {
	pool := mustPool(t)
	ctx := context.Background()
	orgID := seedOrg(t, ctx, pool)
	q := postbacklog.NewPostgresQueue(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Seeded straight into 'processing' — see eventqueue's reap test.
	clickID := idgen.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO postback_attempt_queue (id, organization_id, payload, status, claimed_at)
		VALUES ($1, $2, jsonb_build_object('ClickID', $3::text), 'processing', now() - interval '10 minutes')`,
		idgen.New(), orgID, clickID,
	); err != nil {
		t.Fatalf("seeding claimed row: %v", err)
	}
	if _, err := q.ReapStuck(ctx, 5*time.Minute); err != nil {
		t.Fatalf("ReapStuck: %v", err)
	}

	var status string
	var claimedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT status, claimed_at FROM postback_attempt_queue WHERE payload->>'ClickID' = $1`, clickID,
	).Scan(&status, &claimedAt); err != nil {
		t.Fatalf("reading row: %v", err)
	}
	if status != "queued" || claimedAt != nil {
		t.Fatalf("status=%s claimed_at=%v, want queued/nil", status, claimedAt)
	}
	_, _ = pool.Exec(ctx, `DELETE FROM postback_attempt_queue WHERE payload->>'ClickID' = $1`, clickID)
}

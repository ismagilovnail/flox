package retention_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/retention"
)

func TestFailedPostbacksExpireSuccessesStay(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("needs DATABASE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	orgID, networkID := idgen.New(), idgen.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pg.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'retention test')`, orgID)
	t.Cleanup(func() { _, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	exec(`INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, networkID, orgID)

	ids := map[string]string{}
	postback := func(name, result, status string, age time.Duration) {
		t.Helper()
		id := idgen.New()
		ids[name] = id
		exec(`INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, created_at, event_at)
		      VALUES ($1, $2, $3, $4, $5, 'incoming', $6, now() - make_interval(secs => $7), now() - make_interval(secs => $7))`,
			id, orgID, networkID, idgen.New(), status, result, age.Seconds())
	}
	const day = 24 * time.Hour
	postback("old success", "success", "CPA_ACCEPT", 400*day)
	postback("old duplicate", "duplicate", "CPA_ACCEPT", 100*day)
	postback("old error", "error", "", 100*day)
	postback("old ignored", "ignored", "CPA_HOLD", 91*day)
	postback("old ignored 2", "ignored", "CPA_HOLD", 95*day)
	postback("young duplicate", "duplicate", "CPA_ACCEPT", 89*day)

	// Batch 2 over (at least) 4 expired rows: the loop must go on until a
	// short batch.
	job := &retention.FailedPostbacks{DB: pg, Age: 90 * day, Batch: 2, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if _, err := job.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	for name, id := range ids {
		var left bool
		if err := pg.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM postbacks WHERE id = $1)`, id).Scan(&left); err != nil {
			t.Fatal(err)
		}
		want := name == "old success" || name == "young duplicate"
		if left != want {
			t.Errorf("%s: present %v, want %v", name, left, want)
		}
	}
}

// TestWaitingPostbacksStayWhileTheyCanBeRecorded: a postback waiting for
// its status mapping outlives a short retention while
// conversion.MappingReplayWindow lasts (audit 2026-10-15, DATA-4).
func TestWaitingPostbacksStayWhileTheyCanBeRecorded(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("needs DATABASE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	orgID, networkID := idgen.New(), idgen.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pg.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'retention test')`, orgID)
	t.Cleanup(func() { _, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	exec(`INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, networkID, orgID)

	const day = 24 * time.Hour
	ids := map[string]string{}
	postback := func(name string, awaiting bool, age time.Duration) {
		t.Helper()
		id := idgen.New()
		ids[name] = id
		exec(`INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result, raw_status, awaiting_mapping, created_at)
		      VALUES ($1, $2, $3, $4, '', 'incoming', 'error', 'approved', $5, now() - make_interval(secs => $6))`,
			id, orgID, networkID, idgen.New(), awaiting, age.Seconds())
	}
	postback("waiting 20 days", true, 20*day)
	postback("waiting 40 days", true, 40*day)
	postback("error 20 days", false, 20*day)

	job := &retention.FailedPostbacks{DB: pg, Age: 10 * day, Organization: orgID, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if _, err := job.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	for name, id := range ids {
		var left bool
		if err := pg.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM postbacks WHERE id = $1)`, id).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if want := name == "waiting 20 days"; left != want {
			t.Errorf("%s: present %v, want %v", name, left, want)
		}
	}
}

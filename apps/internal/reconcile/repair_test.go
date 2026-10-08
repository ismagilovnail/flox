package reconcile_test

import (
	"context"
	"io"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/reconcile"
)

func TestRepairMakesClickHouseMatchPostgres(t *testing.T) {
	dsn, chURL := os.Getenv("DATABASE_URL"), os.Getenv("CLICKHOUSE_URL")
	if dsn == "" || chURL == "" {
		t.Skip("needs DATABASE_URL and CLICKHOUSE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(pg.Close) // not defer: the org cleanup below runs in t.Cleanup and needs the pool
	ch, err := chconn.NewConn(ctx, config.ClickHouseConfig{URL: chURL, Database: "flox", User: "flox"})
	if err != nil {
		t.Fatalf("clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = ch.Close() })
	if err := chstore.Migrate(ctx, ch); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := chstore.NewEventStore(ch)

	orgID, networkID, campaignID := idgen.New(), idgen.New(), idgen.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pg.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'repair test')`, orgID)
	t.Cleanup(func() { _, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID) })
	exec(`INSERT INTO networks (id, organization_id, name, accept_duplicates) VALUES ($1, $2, 'n', true)`, networkID, orgID)

	day := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	at := day.Add(12 * time.Hour)

	// The attributed click, so a re-emitted event gets its campaign back.
	clickID := idgen.New()
	if err := store.InsertBatch(chstore.WithBatchID(ctx, idgen.New()), []event.Event{{
		ID: idgen.New(), Type: event.SourceClick, EventAt: at.Add(-time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID, Country: "DE",
	}}); err != nil {
		t.Fatalf("seeding the click: %v", err)
	}

	row := func(status string, usd *float64) string {
		t.Helper()
		id := idgen.New()
		exec(`INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result,
		        network_accepts_duplicates, usd_value, revenue, currency, attribution_outcome, attributed_click_id, event_at)
		      VALUES ($1, $2, $3, $4, $5, 'incoming', 'success', true, $6, $6, 'USD', 'attributed', $4, $7)`,
			id, orgID, networkID, clickID, status, usd, at)
		return id
	}
	ce := func(id string, status event.Type, usd float64) event.Event {
		return event.Event{ID: id, Type: status, EventAt: at, OrganizationID: orgID, CampaignID: campaignID, ClickID: clickID,
			USDValue: usd, HasUSDValue: usd != 0}
	}
	v := func(x float64) *float64 { return &x }

	missing := row("CPA_ACCEPT", v(25))
	dup := row("CPA_REDEP", v(10))
	lateUSD := row("CPA_HOLD", v(5)) // ClickHouse got it before its USD value existed
	if err := store.InsertBatch(chstore.WithBatchID(ctx, idgen.New()), []event.Event{
		ce(dup, event.CpaRedep, 10), ce(dup, event.CpaRedep, 10),
		ce(lateUSD, event.CpaHold, 0),
		ce(idgen.New(), event.CpaRedep, 99), // an orphan Postgres never recorded
		ce("", event.CpaHold, 0),            // predates event ids
	}); err != nil {
		t.Fatalf("seeding clickhouse: %v", err)
	}

	r := reconcile.New(pg, ch, slog.New(slog.NewTextHandler(io.Discard, nil)))
	from, to := day, day.AddDate(0, 0, 1)
	before, err := r.Compare(ctx, from, to)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	var mine []reconcile.Mismatch
	for _, m := range before {
		if m.OrganizationID == orgID {
			mine = append(mine, m)
		}
	}
	if len(mine) != 3 {
		t.Fatalf("mismatched buckets before repair = %+v, want ACCEPT, REDEP and HOLD", mine)
	}

	got, err := r.Repair(ctx, mine, from, to)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if got.Reemitted != 3 || got.Deleted != 3 || got.Unmatched != 1 {
		t.Fatalf("repair = %+v, want 3 re-emitted (missing, dup, late USD), 3 deleted (dup, late USD, orphan), 1 unmatched", got)
	}

	after, err := r.Compare(ctx, from, to)
	if err != nil {
		t.Fatalf("Compare after: %v", err)
	}
	for _, m := range after {
		if m.OrganizationID == orgID && m.Status != "CPA_HOLD" {
			t.Fatalf("still differs after repair: %+v", m)
		}
	}

	// The re-emitted accept carries its click's campaign; LTV and the
	// daily aggregate agree with the corrected events.
	var campaign string
	if err := ch.QueryRow(ctx, `SELECT campaign_id FROM conversion_events WHERE organization_id = ? AND event_id = ?`, orgID, missing).Scan(&campaign); err != nil {
		t.Fatalf("reading the re-emitted accept: %v", err)
	}
	if campaign != campaignID {
		t.Fatalf("re-emitted accept campaign = %q, want %q (restored from click_lookup)", campaign, campaignID)
	}
	sum := func(q string) float64 {
		t.Helper()
		var x float64
		if err := ch.QueryRow(ctx, q, orgID).Scan(&x); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return x
	}
	src := sum(`SELECT sum(usd_value * has_usd_value) FROM conversion_events WHERE organization_id = ?`)
	ltv := sum(`SELECT sum(usd_value * has_usd_value) FROM ltv_events WHERE organization_id = ?`)
	daily := sum(`SELECT sum(revenue_usd) FROM conversion_events_daily_campaign WHERE organization_id = ?`)
	if src != 40 || math.Abs(ltv-src) > 1e-9 || math.Abs(daily-src) > 1e-9 {
		t.Fatalf("usd: events %v, ltv %v, daily %v — want all 40 (25 + 10 + 5)", src, ltv, daily)
	}
}

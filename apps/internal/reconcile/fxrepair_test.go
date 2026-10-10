package reconcile_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/fx"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
	"github.com/ismagilovnail/flox/apps/internal/reconcile"
)

// fxCurrency: an ISO code no provider publishes and no other test uses
// (internal/fx's tests own XTS and XXX), so parallel packages and the dev
// worker's real sync leave these rows alone.
const fxCurrency = "XBB"

type fixedRates []fx.Rate

func (f fixedRates) Name() string { return "fixed" }
func (f fixedRates) Rates(context.Context, time.Time) ([]fx.Rate, error) {
	return f, nil
}

// A rate that arrives after the fact reaches the reports for a day far
// outside the hourly reconciliation's window: the sync repairs Postgres and
// rewrites the ClickHouse copy (tech audit AN1).
func TestLateFXRateRewritesOldConversionsInClickHouse(t *testing.T) {
	dsn, chURL := os.Getenv("DATABASE_URL"), os.Getenv("CLICKHOUSE_URL")
	if dsn == "" || chURL == "" {
		t.Skip("needs DATABASE_URL and CLICKHOUSE_URL")
	}
	ctx := context.Background()
	pg, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(pg.Close)
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
	exec(`INSERT INTO organizations (id, name) VALUES ($1, 'late fx test')`, orgID)
	t.Cleanup(func() {
		_, _ = pg.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = pg.Exec(context.Background(), `DELETE FROM fx_rates WHERE currency = $1`, fxCurrency)
	})
	exec(`INSERT INTO networks (id, organization_id, name) VALUES ($1, $2, 'n')`, networkID, orgID)

	// 50 days ago: the hourly reconciliation (last 3 days) never looks here.
	now := time.Now().UTC()
	day := now.AddDate(0, 0, -50).Truncate(24 * time.Hour)
	at := day.Add(12 * time.Hour)

	clickID, acceptID := idgen.New(), idgen.New()
	if err := store.InsertBatch(chstore.WithBatchID(ctx, idgen.New()), []event.Event{{
		ID: idgen.New(), Type: event.SourceClick, EventAt: at.Add(-time.Hour), OrganizationID: orgID,
		CampaignID: campaignID, ClickID: clickID, Country: "DE",
	}}); err != nil {
		t.Fatalf("seeding the click: %v", err)
	}
	// Recorded with no rate on file: no USD value in either store.
	exec(`INSERT INTO postbacks (id, organization_id, network_id, click_id, status, direction, result,
	        revenue, currency, attribution_outcome, attributed_click_id, event_at, created_at)
	      VALUES ($1, $2, $3, $4, 'CPA_ACCEPT', 'incoming', 'success', 10, $5, 'attributed', $4, $6, $6)`,
		acceptID, orgID, networkID, clickID, fxCurrency, at)
	if err := store.InsertBatch(chstore.WithBatchID(ctx, idgen.New()), []event.Event{{
		ID: acceptID, Type: event.CpaAccept, EventAt: at, OrganizationID: orgID, CampaignID: campaignID,
		ClickID: clickID, Revenue: 10, Currency: fxCurrency,
	}}); err != nil {
		t.Fatalf("seeding the conversion: %v", err)
	}

	reconciler := reconcile.New(pg, ch, slog.New(slog.NewTextHandler(io.Discard, nil)))
	syncer := fx.NewSyncer(fixedRates{{Currency: fxCurrency, Date: day, RateToUSD: 4}}, fx.NewStore(pg),
		slog.New(slog.NewTextHandler(io.Discard, nil))).
		OnRepaired(func(ctx context.Context, buckets []fx.Bucket) error {
			bs := make([]reconcile.Bucket, len(buckets))
			for i, b := range buckets {
				bs[i] = reconcile.Bucket{OrganizationID: b.OrganizationID, Day: b.Day, Status: b.Status}
			}
			_, err := reconciler.RepairBuckets(ctx, bs)
			return err
		})
	if err := syncer.RunOnce(ctx); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var usd float64
	var copies uint64
	if err := ch.QueryRow(ctx, `
		SELECT sum(usd_value * has_usd_value), count() FROM conversion_events
		WHERE organization_id = ? AND event_id = ?`, orgID, acceptID).Scan(&usd, &copies); err != nil {
		t.Fatal(err)
	}
	if copies != 1 || usd != 40 {
		t.Fatalf("clickhouse copy after the late rate: %d copies, usd %v — want 1 copy, 40", copies, usd)
	}
	var daily float64
	if err := ch.QueryRow(ctx, `SELECT sum(revenue_usd) FROM conversion_events_daily_campaign WHERE organization_id = ?`, orgID).Scan(&daily); err != nil {
		t.Fatal(err)
	}
	if daily != 40 {
		t.Fatalf("daily aggregate revenue = %v, want 40", daily)
	}
}

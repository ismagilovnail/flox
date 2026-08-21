package chstore_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func mustConn(t *testing.T) driver.Conn {
	t.Helper()
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		t.Skip("CLICKHOUSE_URL not set, skipping integration test")
	}
	conn, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{
		URL:      url,
		Database: envOr("CLICKHOUSE_DATABASE", "flox"),
		User:     envOr("CLICKHOUSE_USER", "flox"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	})
	if err != nil {
		t.Fatalf("connecting to clickhouse: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := chstore.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("migrating schema: %v", err)
	}
	return conn
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// TestInsertBatchRoutesByType is the money test for §48's three-way split:
// one mixed batch (click + tracking + conversion) must land each row in
// its own table, not all in one.
func TestInsertBatchRoutesByType(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	campaignID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	events := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US"},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US"},
		{Type: event.SourceFilter, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), FilterReason: "bot"},
		{Type: event.LandView, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.PwaInstall, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Revenue: 50, Currency: "USD", USDValue: 50, HasUSDValue: true},
		{Type: event.CpaHold, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	var clickCount, trackingCount, conversionCount uint64
	if err := conn.QueryRow(ctx, `SELECT count() FROM click_events WHERE organization_id = ?`, orgID).Scan(&clickCount); err != nil {
		t.Fatalf("counting click_events: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT count() FROM tracking_events WHERE organization_id = ?`, orgID).Scan(&trackingCount); err != nil {
		t.Fatalf("counting tracking_events: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT count() FROM conversion_events WHERE organization_id = ?`, orgID).Scan(&conversionCount); err != nil {
		t.Fatalf("counting conversion_events: %v", err)
	}
	if clickCount != 3 {
		t.Fatalf("click_events count = %d, want 3 (2 SOURCE_CLICK + 1 SOURCE_FILTER)", clickCount)
	}
	if trackingCount != 2 {
		t.Fatalf("tracking_events count = %d, want 2 (LAND_VIEW + PWA_INSTALL)", trackingCount)
	}
	if conversionCount != 2 {
		t.Fatalf("conversion_events count = %d, want 2 (CPA_ACCEPT + CPA_HOLD)", conversionCount)
	}

	// Materialized views fire synchronously with the INSERT in ClickHouse
	// (the MV's SELECT runs over the just-inserted block before Send
	// returns), so no polling/sleep is needed here — but merges of
	// SummingMergeTree rows are NOT synchronous, which is exactly why the
	// queries below sum themselves rather than trusting one row per key.
	counts, err := store.DailyCampaignCounts(ctx, orgID, campaignID, day, day)
	if err != nil {
		t.Fatalf("DailyCampaignCounts: %v", err)
	}
	byType := map[event.Type]uint64{}
	for _, c := range counts {
		byType[c.Type] += c.EventCount
	}
	if byType[event.SourceClick] != 2 {
		t.Fatalf("click_events_daily_campaign SOURCE_CLICK = %d, want 2 (got %+v)", byType[event.SourceClick], counts)
	}
	if byType[event.SourceFilter] != 1 {
		t.Fatalf("click_events_daily_campaign SOURCE_FILTER = %d, want 1 (got %+v)", byType[event.SourceFilter], counts)
	}
	if _, ok := byType[event.CpaAccept]; ok {
		t.Fatal("click_events_daily_campaign must not contain CPA_ACCEPT — that's conversion_events_daily_campaign's row")
	}

	revenue, err := store.DailyCampaignRevenue(ctx, orgID, campaignID, day, day)
	if err != nil {
		t.Fatalf("DailyCampaignRevenue: %v", err)
	}
	var acceptRevenue float64
	var acceptCount, holdCount uint64
	for _, r := range revenue {
		if r.Type == event.CpaAccept {
			acceptRevenue += r.RevenueUSD
			acceptCount += r.EventCount
		}
		if r.Type == event.CpaHold {
			holdCount += r.EventCount
		}
	}
	if acceptCount != 1 || acceptRevenue != 50 {
		t.Fatalf("CPA_ACCEPT aggregate = count %d revenue %v, want count 1 revenue 50", acceptCount, acceptRevenue)
	}
	if holdCount != 1 {
		t.Fatalf("CPA_HOLD count = %d, want 1", holdCount)
	}
}

func TestDailyCampaignGeoAggregate(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	events := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, ClickID: idgen.New(), Country: "US"},
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, ClickID: idgen.New(), Country: "US"},
		{Type: event.SourceClick, EventAt: day.Add(time.Hour), OrganizationID: orgID, ClickID: idgen.New(), Country: "DE"},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	rows, err := conn.Query(ctx, `
		SELECT country, sum(event_count) FROM click_events_daily_geo
		WHERE organization_id = ? AND day = ? GROUP BY country`,
		orgID, day.Format("2006-01-02"),
	)
	if err != nil {
		t.Fatalf("querying click_events_daily_geo: %v", err)
	}
	defer rows.Close()

	byCountry := map[string]uint64{}
	for rows.Next() {
		var country string
		var count uint64
		if err := rows.Scan(&country, &count); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		byCountry[country] = count
	}
	if byCountry["US"] != 2 || byCountry["DE"] != 1 {
		t.Fatalf("click_events_daily_geo = %+v, want US=2 DE=1", byCountry)
	}
}

func TestInsertBatchEmptyIsNoop(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	if err := store.InsertBatch(context.Background(), nil); err != nil {
		t.Fatalf("InsertBatch(nil): %v", err)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	conn := mustConn(t)
	if err := chstore.Migrate(context.Background(), conn); err != nil {
		t.Fatalf("second Migrate call: %v", err)
	}
}

func TestMigrateDropsPhase25Schema(t *testing.T) {
	conn := mustConn(t)
	var count uint64
	err := conn.QueryRow(context.Background(), `
		SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = 'events'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying system.tables: %v", err)
	}
	if count != 0 {
		t.Fatal("Phase 25's disposable `events` table still exists after Migrate")
	}
}

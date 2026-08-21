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

func TestInsertBatchAndDailyAggregate(t *testing.T) {
	conn := mustConn(t)
	store := chstore.NewEventStore(conn)
	ctx := context.Background()

	orgID := idgen.New()
	campaignID := idgen.New()
	day := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	events := []event.Event{
		{Type: event.SourceClick, EventAt: day.Add(1 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.SourceClick, EventAt: day.Add(2 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New()},
		{Type: event.CpaAccept, EventAt: day.Add(3 * time.Hour), OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Revenue: 50, Currency: "USD", USDValue: 50, HasUSDValue: true},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	// Materialized views fire synchronously with the INSERT in ClickHouse
	// (the MV's SELECT runs over the just-inserted block before Send
	// returns), so no polling/sleep is needed here — but merges of
	// SummingMergeTree rows are NOT synchronous, which is exactly why the
	// query sums itself rather than trusting one row per key.
	counts, err := store.DailyCampaignCounts(ctx, orgID, campaignID, day, day)
	if err != nil {
		t.Fatalf("DailyCampaignCounts: %v", err)
	}

	byType := map[event.Type]uint64{}
	for _, c := range counts {
		byType[c.Type] += c.EventCount
	}
	if byType[event.SourceClick] != 2 {
		t.Fatalf("SOURCE_CLICK count = %d, want 2 (got %+v)", byType[event.SourceClick], counts)
	}
	if byType[event.CpaAccept] != 1 {
		t.Fatalf("CPA_ACCEPT count = %d, want 1 (got %+v)", byType[event.CpaAccept], counts)
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

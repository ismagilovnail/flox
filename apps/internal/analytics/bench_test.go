package analytics_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/analytics"
	"github.com/ismagilovnail/flox/apps/internal/chconn"
	"github.com/ismagilovnail/flox/apps/internal/chstore"
	"github.com/ismagilovnail/flox/apps/internal/config"
	"github.com/ismagilovnail/flox/apps/internal/event"
	"github.com/ismagilovnail/flox/apps/internal/idgen"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// benchAnalyticsFixture inserts 30 days x ~200 clicks/day (some filtered,
// some converting) for one campaign into real ClickHouse, through
// chstore.EventStore.InsertBatch — the same write path apps/worker uses —
// so the two aggregate queries below read from materialized data at a
// size representative of a real campaign's monthly volume, not an empty
// table.
func benchAnalyticsFixture(b *testing.B) (svc *analytics.Service, orgID, campaignID string, from, to time.Time) {
	b.Helper()
	url := os.Getenv("CLICKHOUSE_URL")
	if url == "" {
		b.Skip("CLICKHOUSE_URL not set, skipping ClickHouse-backed benchmark")
	}
	conn, err := chconn.NewConn(context.Background(), config.ClickHouseConfig{
		URL:      url,
		Database: envOr("CLICKHOUSE_DATABASE", "flox"),
		User:     envOr("CLICKHOUSE_USER", "flox"),
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
	})
	if err != nil {
		b.Fatalf("connecting to clickhouse: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close() })
	if err := chstore.Migrate(context.Background(), conn); err != nil {
		b.Fatalf("migrating schema: %v", err)
	}

	store := chstore.NewEventStore(conn)
	orgID = idgen.New()
	campaignID = idgen.New()
	from = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to = from.AddDate(0, 0, 29)

	const perDay = 200
	events := make([]event.Event, 0, 30*perDay)
	for d := 0; d < 30; d++ {
		day := from.AddDate(0, 0, d)
		for i := 0; i < perDay; i++ {
			switch {
			case i%20 == 0:
				events = append(events, event.Event{
					Type: event.CpaAccept, EventAt: day.Add(time.Duration(i) * time.Minute),
					OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(),
					Revenue: 25.5, Currency: "USD", USDValue: 25.5, HasUSDValue: true,
				})
			case i%10 == 0:
				events = append(events, event.Event{
					Type: event.SourceFilter, EventAt: day.Add(time.Duration(i) * time.Minute),
					OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), FilterReason: "geo",
				})
			default:
				events = append(events, event.Event{
					Type: event.SourceClick, EventAt: day.Add(time.Duration(i) * time.Minute),
					OrganizationID: orgID, CampaignID: campaignID, ClickID: idgen.New(), Country: "US",
				})
			}
		}
	}
	if err := store.InsertBatch(context.Background(), events); err != nil {
		b.Fatalf("seeding events: %v", err)
	}
	b.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, table := range []string{
			"click_events", "conversion_events",
			"click_events_daily_campaign", "click_events_daily_geo", "conversion_events_daily_campaign",
		} {
			if err := conn.Exec(cleanupCtx, "ALTER TABLE "+table+" DELETE WHERE organization_id = ?", orgID); err != nil {
				b.Logf("cleaning up %s: %v", table, err)
			}
		}
	})

	return analytics.NewService(store), orgID, campaignID, from, to
}

// BenchmarkCampaignDaily and BenchmarkCampaignDailyRevenue measure
// apps/internal/analytics.Service's two read endpoints against a real
// ClickHouse over a full 30-day range — Phase 31's benchmark list (§56)
// names "analytics" alongside the tracking/routing/classifier/postback
// hot-path pieces, even though (like postback) it carries no explicit
// p50/p95 target: these run off apps/api's request path, not apps/
// tracker's, and are already latency-instrumented in production via
// metrics.AnalyticsQueryLatencySeconds.
func BenchmarkCampaignDaily(b *testing.B) {
	svc, orgID, campaignID, from, to := benchAnalyticsFixture(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.CampaignDaily(ctx, orgID, campaignID, from, to); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCampaignDailyRevenue(b *testing.B) {
	svc, orgID, campaignID, from, to := benchAnalyticsFixture(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.CampaignDailyRevenue(ctx, orgID, campaignID, from, to); err != nil {
			b.Fatal(err)
		}
	}
}

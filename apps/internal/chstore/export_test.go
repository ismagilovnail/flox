package chstore

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// NewClickResolverWithBackfill lets tests pick the backfill marker the
// resolver waits for, instead of touching the shared one live services use.
func NewClickResolverWithBackfill(conn driver.Conn, name string) *ClickResolver {
	return &ClickResolver{conn: conn, lookupBackfill: name}
}

// RunBackfill runs one ad-hoc backfill through the real runner.
func RunBackfill(ctx context.Context, conn driver.Conn, name, sql string) error {
	return runBackfills(ctx, conn, []backfill{{name: name, sql: sql}})
}

// SyncMutations makes ALTER ... DELETE in tests wait until applied.
func SyncMutations(ctx context.Context) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
}

// RunNetworkBackfill runs BackfillNetworks under its own name.
func RunNetworkBackfill(ctx context.Context, conn driver.Conn, name string, offers map[string]string) error {
	return backfillNetworks(ctx, conn, name, func(context.Context) (map[string]string, error) { return offers, nil })
}

// RunLTVAttributionBackfill runs BackfillLTVAttribution under its own name.
func RunLTVAttributionBackfill(ctx context.Context, conn driver.Conn, name string) error {
	return backfillLTVAttribution(ctx, conn, name)
}

// RunVisitorKeyBackfill runs BackfillVisitorKeys under its own name, on
// one organization's clicks.
func RunVisitorKeyBackfill(ctx context.Context, conn driver.Conn, name string, keyOf func(ip, userAgent string) string, orgID string) error {
	return backfillVisitorKeys(ctx, conn, name, keyOf, orgID)
}

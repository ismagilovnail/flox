package chstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// SetPostbackLogTTL makes postback_events — the log of every postback
// attempt, in and out — expire after months; 0 removes the TTL (tech
// audit CH3, docs/retention.md). Only this log: the report tables
// (click_events, tracking_events, conversion_events, ltv_events,
// click_lookup) are what every report reads for any period, and are kept.
//
// Run by apps/worker at start, not a schema file: the period is
// configuration (RETENTION_POSTBACK_LOG_MONTHS). It changes the table only
// when the TTL differs — a MODIFY TTL rewrites the existing parts.
func SetPostbackLogTTL(ctx context.Context, conn driver.Conn, months int) (changed bool, err error) {
	var engine string
	if err := conn.QueryRow(ctx, `
		SELECT engine_full FROM system.tables WHERE database = currentDatabase() AND name = 'postback_events'`).Scan(&engine); err != nil {
		return false, fmt.Errorf("chstore: reading postback_events engine: %w", err)
	}
	want := ""
	if months > 0 {
		want = fmt.Sprintf("TTL toDateTime(event_at) + toIntervalMonth(%d)", months)
	}
	has := strings.Contains(engine, " TTL ")
	switch {
	case want == "" && !has, want != "" && strings.Contains(engine, want):
		return false, nil
	case want == "":
		err = conn.Exec(ctx, `ALTER TABLE postback_events REMOVE TTL`)
	default:
		err = conn.Exec(ctx, fmt.Sprintf(`ALTER TABLE postback_events MODIFY TTL toDateTime(event_at) + INTERVAL %d MONTH`, months))
	}
	if err != nil {
		return false, fmt.Errorf("chstore: setting postback_events TTL: %w", err)
	}
	return true, nil
}

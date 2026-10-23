package chstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2"
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

// personalDataTables hold visitors' raw IP addresses, user agents and
// referrers.
var personalDataTables = []string{"click_events", "tracking_events", "click_lookup"}

// personalDataColumns are the columns that expire; a table without one
// (click_lookup has no referrer) is skipped for it. referrer can carry
// the referring page's query string — an email, a search, an id.
var personalDataColumns = []string{"ip", "user_agent", "referrer"}

// SetPersonalDataTTL makes visitors' ip, user_agent and referrer expire
// after days in every table that holds them — a column TTL: the row stays,
// the values are reset to '' (audit 2026-10-15, GDPR-1; docs/retention.md).
// 0 removes the TTL. Unique clicks count the keyed pseudonym (schema/020),
// which has no TTL, so reports do not change when the values go.
//
// Run by apps/worker at start, like SetPostbackLogTTL; a column is altered
// only when its TTL differs — the change rewrites the existing parts.
// ClickHouse applies the TTL when it merges parts, so a value may outlive
// its day by hours.
func SetPersonalDataTTL(ctx context.Context, conn driver.Conn, days int) (changed []string, err error) {
	for _, table := range personalDataTables {
		var create string
		if err := conn.QueryRow(ctx, `
			SELECT create_table_query FROM system.tables WHERE database = currentDatabase() AND name = ?`, table).Scan(&create); err != nil {
			return changed, fmt.Errorf("chstore: reading %s definition: %w", table, err)
		}
		for _, col := range personalDataColumns {
			if !strings.Contains(create, "`"+col+"` String") {
				continue
			}
			want := fmt.Sprintf("`%s` String TTL toDateTime(event_at) + toIntervalDay(%d)", col, days)
			has := strings.Contains(create, "`"+col+"` String TTL ")
			var stmt string
			switch {
			case days == 0 && !has, days > 0 && strings.Contains(create, want):
				continue
			case days == 0:
				stmt = fmt.Sprintf(`ALTER TABLE %s MODIFY COLUMN %s REMOVE TTL`, table, col)
			default:
				stmt = fmt.Sprintf(`ALTER TABLE %s MODIFY COLUMN %s String TTL toDateTime(event_at) + INTERVAL %d DAY`, table, col, days)
			}
			if err := conn.Exec(ctx, stmt); err != nil {
				return changed, fmt.Errorf("chstore: setting %s.%s TTL: %w", table, col, err)
			}
			changed = append(changed, table+"."+col)
		}
	}
	return changed, nil
}

// rawRetentionTables carry retention_months (schema/024).
var rawRetentionTables = []string{"click_events", "tracking_events", "click_lookup"}

// SetRawRetention rewrites retention_months on the organizations' raw
// clicks and tracking events (audit DATA-6), and waits for it. Each
// table's TTL then deletes the rows older than that at its next merges —
// irreversibly; click_lookup keeps a click at least 13 months anyway.
// internal/retention.Plans decides when.
func SetRawRetention(ctx context.Context, conn driver.Conn, months uint8, organizationIDs []string) error {
	if len(organizationIDs) == 0 {
		return nil
	}
	wait := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
	for _, table := range rawRetentionTables {
		if err := conn.Exec(wait, `ALTER TABLE `+table+` UPDATE retention_months = ?
			WHERE organization_id IN ? AND retention_months != ?`, months, organizationIDs, months); err != nil {
			return fmt.Errorf("chstore: setting retention on %s: %w", table, err)
		}
	}
	return nil
}

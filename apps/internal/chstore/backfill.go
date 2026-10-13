package chstore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// backfills are one-time copies of existing data into a table added by a
// later schema file. Schema files run on every start of every service; a
// backfill runs once, from apps/worker only (Backfill), and records itself
// in chstore_backfills. If two runs race on a first start, the target's
// ReplacingMergeTree collapses the duplicate rows.
type backfill struct{ name, sql string }

const clickLookupBackfill = "click_lookup_v1"

// clickLookupColumns names every click_lookup column, so the copy does not
// depend on the table's column order (schema/015 appended some).
const clickLookupColumns = `organization_id, click_id, event_at,
	campaign_id, stream_set_id, flow_id, external_click_id,
	country, region, city, device, platform, os,
	utm_source, utm_medium, utm_campaign, utm_content, utm_term,
	sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
	landing_id, offer_id, pwa_id, traffic_source_id,
	ip, user_agent, fb_click_id, tt_click_id,
	postlanding_id`

var backfills = []backfill{
	{
		name: clickLookupBackfill,
		sql: `INSERT INTO click_lookup (` + clickLookupColumns + `)
			SELECT ` + clickLookupColumns + `
			FROM click_events
			WHERE type = 'SOURCE_CLICK'`,
	},
}

// Backfill runs every backfill not yet recorded. apps/worker calls it once
// at startup, after Migrate.
func Backfill(ctx context.Context, conn driver.Conn) error {
	return runBackfills(ctx, conn, backfills)
}

func runBackfills(ctx context.Context, conn driver.Conn, list []backfill) error {
	for _, b := range list {
		done, err := backfillDone(ctx, conn, b.name)
		if err != nil {
			return err
		}
		if done {
			continue
		}
		if err := conn.Exec(ctx, b.sql); err != nil {
			return fmt.Errorf("chstore: backfill %s: %w", b.name, err)
		}
		if err := conn.Exec(ctx, `INSERT INTO chstore_backfills (name) VALUES (?)`, b.name); err != nil {
			return fmt.Errorf("chstore: recording backfill %s: %w", b.name, err)
		}
	}
	return nil
}

func backfillDone(ctx context.Context, conn driver.Conn, name string) (bool, error) {
	var n uint64
	if err := conn.QueryRow(ctx, `SELECT count() FROM chstore_backfills WHERE name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("chstore: checking backfill %s: %w", name, err)
	}
	return n > 0, nil
}

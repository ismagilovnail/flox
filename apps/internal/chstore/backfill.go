package chstore

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
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

const networkBackfill = "network_id_v1"

// BackfillNetworks gives clicks and tracking events from before
// schema/017 their network, from their offer_id and the offer's network
// now (offers, keyed by offer id, from Postgres — load is only called when
// the backfill has not run yet). An offer moved to another network since
// puts its old clicks on the new one; clicks without an offer stay empty.
// Rows that already have a network are left alone. One map for every
// organization: offer ids are globally unique, and a click only ever
// carries an offer of its own organization. Runs once, from
// apps/worker; it waits for the mutations to finish before recording
// itself, so an interrupted run is redone on the next start.
func BackfillNetworks(ctx context.Context, conn driver.Conn, load func(context.Context) (map[string]string, error)) error {
	return backfillNetworks(ctx, conn, networkBackfill, load)
}

func backfillNetworks(ctx context.Context, conn driver.Conn, name string, load func(context.Context) (map[string]string, error)) error {
	done, err := backfillDone(ctx, conn, name)
	if err != nil || done {
		return err
	}
	offers, err := load(ctx)
	if err != nil {
		return fmt.Errorf("chstore: backfill %s: loading offers: %w", name, err)
	}
	if len(offers) > 0 {
		ids := make([]string, 0, len(offers))
		nets := make([]string, 0, len(offers))
		for id, net := range offers {
			ids = append(ids, id)
			nets = append(nets, net)
		}
		wait := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
		for _, table := range []string{"click_events", "tracking_events"} {
			if err := conn.Exec(wait, `ALTER TABLE `+table+`
				UPDATE network_id = transform(offer_id, ?, ?, '')
				WHERE network_id = '' AND offer_id IN ?`, ids, nets, ids); err != nil {
				return fmt.Errorf("chstore: backfill %s on %s: %w", name, table, err)
			}
		}
	}
	if err := conn.Exec(ctx, `INSERT INTO chstore_backfills (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("chstore: recording backfill %s: %w", name, err)
	}
	return nil
}

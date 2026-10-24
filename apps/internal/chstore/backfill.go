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
	postlanding_id, retention_months,
	ad_click_param, ad_click_id`

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

const ltvAttributionBackfill = "ltv_offer_source_v1"

// BackfillLTVAttribution gives ltv_events rows from before schema/018
// their click's offer and traffic source, from click_lookup. Rows that
// already have either are left alone; a click without an offer (redirect
// flow, fallback) stays empty. Runs once, from apps/worker; it waits for
// the mutation before recording itself, so an interrupted run is redone
// on the next start.
func BackfillLTVAttribution(ctx context.Context, conn driver.Conn) error {
	return backfillLTVAttribution(ctx, conn, ltvAttributionBackfill)
}

func backfillLTVAttribution(ctx context.Context, conn driver.Conn, name string) error {
	done, err := backfillDone(ctx, conn, name)
	if err != nil || done {
		return err
	}
	// A scratch Join table read through joinGet, only while this runs. The
	// mutation runs outside this session, so joinGet needs the database.
	var db string
	if err := conn.QueryRow(ctx, `SELECT currentDatabase()`).Scan(&db); err != nil {
		return fmt.Errorf("chstore: backfill %s: %w", name, err)
	}
	join := "backfill_attrs_" + name
	if err := conn.Exec(ctx, `DROP TABLE IF EXISTS `+join); err != nil {
		return fmt.Errorf("chstore: backfill %s: %w", name, err)
	}
	defer func() { _ = conn.Exec(context.WithoutCancel(ctx), `DROP TABLE IF EXISTS `+join) }()
	if err := conn.Exec(ctx, `
		CREATE TABLE `+join+` ENGINE = Join(ANY, LEFT, organization_id, click_id) AS
		SELECT organization_id, click_id, argMax(offer_id, event_at) AS offer_id, argMax(traffic_source_id, event_at) AS traffic_source_id
		FROM click_lookup
		WHERE (organization_id, click_id) IN (
			SELECT organization_id, click_id FROM ltv_events WHERE offer_id = '' AND traffic_source_id = '')
		GROUP BY organization_id, click_id`); err != nil {
		return fmt.Errorf("chstore: backfill %s: loading clicks: %w", name, err)
	}
	wait := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
	qualified := db + "." + join
	if err := conn.Exec(wait, `ALTER TABLE ltv_events UPDATE
		offer_id = joinGet('`+qualified+`', 'offer_id', organization_id, click_id),
		traffic_source_id = joinGet('`+qualified+`', 'traffic_source_id', organization_id, click_id)
		WHERE offer_id = '' AND traffic_source_id = ''`); err != nil {
		// A failed mutation is retried by ClickHouse forever; stop it, the
		// next start runs the backfill again.
		_ = conn.Exec(context.WithoutCancel(ctx), `KILL MUTATION WHERE database = ? AND table = 'ltv_events' AND NOT is_done AND command LIKE ?`,
			db, "%"+join+"%")
		return fmt.Errorf("chstore: backfill %s: %w", name, err)
	}
	if err := conn.Exec(ctx, `INSERT INTO chstore_backfills (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("chstore: recording backfill %s: %w", name, err)
	}
	return nil
}

const visitorKeyBackfill = "visitor_key_v1"

// visitorKeyBatch is how many distinct IP + user agent pairs one mutation
// carries.
const visitorKeyBatch = 2000

// BackfillVisitorKeys gives clicks from before schema/020 their visitor
// pseudonym (internal/visitorkey), from their IP and user agent while they
// are still kept: once RETENTION_IP_DAYS wipes those, a click without a
// pseudonym would count as its own unique visitor. keyOf is
// visitorkey.Of with the key; nil (VISITOR_KEY unset) skips the backfill
// without recording it, so it runs once a key is configured. Runs once,
// from apps/worker; it waits for each mutation, so an interrupted run is
// picked up where it stopped (rows that have a key are left alone).
func BackfillVisitorKeys(ctx context.Context, conn driver.Conn, keyOf func(ip, userAgent string) string) error {
	return backfillVisitorKeys(ctx, conn, visitorKeyBackfill, keyOf, "")
}

// backfillVisitorKeys with orgID limits the run to one organization — for
// tests, which share the tables with the dev stack; "" is every one.
func backfillVisitorKeys(ctx context.Context, conn driver.Conn, name string, keyOf func(ip, userAgent string) string, orgID string) error {
	if keyOf == nil {
		return nil
	}
	done, err := backfillDone(ctx, conn, name)
	if err != nil || done {
		return err
	}
	rows, err := conn.Query(ctx, `
		SELECT DISTINCT ip, user_agent FROM click_events
		WHERE visitor_key = '' AND (ip != '' OR user_agent != '') AND (? = '' OR organization_id = ?)`, orgID, orgID)
	if err != nil {
		return fmt.Errorf("chstore: backfill %s: reading visitors: %w", name, err)
	}
	var visitors, keys []string
	for rows.Next() {
		var ip, ua string
		if err := rows.Scan(&ip, &ua); err != nil {
			rows.Close()
			return fmt.Errorf("chstore: backfill %s: reading visitors: %w", name, err)
		}
		visitors = append(visitors, ip+"\x00"+ua)
		keys = append(keys, keyOf(ip, ua))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("chstore: backfill %s: reading visitors: %w", name, err)
	}
	wait := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
	for start := 0; start < len(visitors); start += visitorKeyBatch {
		end := min(start+visitorKeyBatch, len(visitors))
		batch, batchKeys := visitors[start:end], keys[start:end]
		if err := conn.Exec(wait, `ALTER TABLE click_events
			UPDATE visitor_key = transform(concat(ip, char(0), user_agent), ?, ?, '')
			WHERE visitor_key = '' AND concat(ip, char(0), user_agent) IN ? AND (? = '' OR organization_id = ?)`,
			batch, batchKeys, batch, orgID, orgID); err != nil {
			return fmt.Errorf("chstore: backfill %s: %w", name, err)
		}
	}
	if err := conn.Exec(ctx, `INSERT INTO chstore_backfills (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("chstore: recording backfill %s: %w", name, err)
	}
	return nil
}

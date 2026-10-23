package chstore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// The hourly rollup (schema/023, audit DATA-6, docs/retention.md
// "Rollup"): clicks and tracking events counted per UTC hour and report
// dimension, so reports outlive the raw rows.

// rollupColumns are the report dimensions event_rollup_hourly keeps —
// reportClickColumns without region and city. Same column names as the
// raw tables, so a query's group-by and filter clauses apply to either.
var rollupColumns = func() map[ReportDimension]string {
	out := map[ReportDimension]string{}
	for d, col := range reportClickColumns {
		if d != DimRegion && d != DimCity {
			out[d] = col
		}
	}
	return out
}()

const rollupBackfill = "event_rollup_v1"

// rollupBackfillDelay: the backfill copies the raw rows from before the
// cutover this long after it, so events still on their way at the cutover
// (the worker's flush, a NATS backlog) are in the raw table by then. An
// event older than the cutover that arrives later still is in no rollup.
const rollupBackfillDelay = time.Hour

// rollupDims lists the rollup's dimension columns, in its sort-key order.
const rollupDims = `campaign_id, traffic_source_id, stream_set_id, flow_id, landing_id, offer_id, pwa_id, postlanding_id,
	network_id, tg_channel_id, country, device, platform, os, browser, language`

// rollupState caches whether the rollup holds every event: once it does,
// it always will; until then it is asked at most once a minute.
type rollupState struct {
	mu      sync.Mutex
	ready   bool
	checked time.Time
}

// useRollup: whether a report with these dimensions and filters over
// [from, to) can read event_rollup_hourly. Its hours must be whole —
// a day in a half-hour time zone is not — and the backfill done.
func (s *EventStore) useRollup(ctx context.Context, dims []ReportDimension, filters []ReportFilter, from, to time.Time) bool {
	for _, d := range dims {
		if _, ok := rollupColumns[d]; !ok {
			return false
		}
	}
	for _, f := range filters {
		if _, ok := rollupColumns[f.Dimension]; !ok {
			return false
		}
	}
	if !wholeHour(from) || !wholeHour(to) {
		return false
	}
	return s.rollupReady(ctx)
}

func wholeHour(t time.Time) bool { return t.Equal(t.Truncate(time.Hour)) }

func (s *EventStore) rollupReady(ctx context.Context) bool {
	if s.rollup == nil {
		return false
	}
	s.rollup.mu.Lock()
	defer s.rollup.mu.Unlock()
	if s.rollup.ready || time.Since(s.rollup.checked) < time.Minute {
		return s.rollup.ready
	}
	s.rollup.checked = time.Now()
	done, err := backfillDone(ctx, s.conn, rollupBackfill)
	s.rollup.ready = err == nil && done
	return s.rollup.ready
}

// rollupClickMetrics is the rollup's metric expressions for a click report; the
// raw table's are in ReportClicks.
const rollupClickMetrics = `sumIf(events, type = 'SOURCE_CLICK') AS clicks,
	       uniqExactMergeIf(visitors, type = 'SOURCE_CLICK') AS unique_clicks,
	       sumIf(events, type = 'SOURCE_FILTER') AS blocked,
	       sum(bots) AS bots,
	       sum(empty_subs) AS empty_subs`

// BackfillRollup copies the clicks and tracking events from before the
// rollup's cutover into it, once, rollupBackfillDelay after the cutover —
// waiting for it if need be, so apps/worker runs it in the background.
// Until it is done, reports read the raw rows. Interrupted, it reruns on
// the next start: the copy runs only when nothing from before the cutover
// is in the rollup yet, so it never counts twice.
func BackfillRollup(ctx context.Context, conn driver.Conn) error {
	return backfillRollup(ctx, conn, rollupBackfill, "", true)
}

// backfillRollup: orgID "" copies every organization; tests copy one, and
// do not wait for the cutover.
func backfillRollup(ctx context.Context, conn driver.Conn, name, orgID string, wait bool) error {
	done, err := backfillDone(ctx, conn, name)
	if err != nil || done {
		return err
	}
	var cutover time.Time
	if err := conn.QueryRow(ctx, `SELECT max(cutover) FROM event_rollup_cutover`).Scan(&cutover); err != nil {
		return fmt.Errorf("chstore: backfill %s: reading the cutover: %w", name, err)
	}
	if d := time.Until(cutover.Add(rollupBackfillDelay)); wait && d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	org, args := "", []any{cutover}
	if orgID != "" {
		org, args = " AND organization_id = ?", append(args, orgID)
	}
	var copied uint64
	if err := conn.QueryRow(ctx, `SELECT count() FROM event_rollup_hourly WHERE event_at < ?`+org, args...).Scan(&copied); err != nil {
		return fmt.Errorf("chstore: backfill %s: %w", name, err)
	}
	if copied == 0 {
		for _, table := range []string{"click_events", "tracking_events"} {
			if err := conn.Exec(ctx, rollupCopy(table, org), args...); err != nil {
				return fmt.Errorf("chstore: backfill %s from %s: %w", name, table, err)
			}
		}
	}
	if err := conn.Exec(ctx, `INSERT INTO chstore_backfills (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("chstore: recording backfill %s: %w", name, err)
	}
	return nil
}

// rollupCopy is the rollup's view (schema/023) over the rows before the
// cutover instead of after it.
func rollupCopy(table, org string) string {
	metrics := `count(), countIf(is_bot = 1), countIf(type = 'SOURCE_CLICK' AND ` + emptySubsCondition + `),
		uniqExactStateIf(` + uniqueVisitorExpr + `, type = 'SOURCE_CLICK')`
	where := `type IN ('SOURCE_CLICK', 'SOURCE_FILTER') AND `
	if table == "tracking_events" {
		metrics = `count(), countIf(is_bot = 1), toUInt64(0), uniqExactStateIf('', 0)`
		where = ""
	}
	dims := strings.Join(strings.Fields(rollupDims), " ")
	return `INSERT INTO event_rollup_hourly (organization_id, event_at, type, ` + dims + `, events, bots, empty_subs, visitors)
		SELECT organization_id, toStartOfHour(event_at) AS hour, type, ` + dims + `, ` + metrics + `
		FROM ` + table + `
		WHERE ` + where + `event_at < ?` + org + `
		GROUP BY organization_id, hour, type, ` + dims
}

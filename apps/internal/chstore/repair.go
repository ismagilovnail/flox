package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Repair primitives for internal/reconcile: find a bucket's conversion
// events by event_id, delete the wrong ones, and rebuild the daily
// aggregate rows they fed. Every statement is scoped by organization_id.

// StoredEvent is one event_id's copies in conversion_events.
type StoredEvent struct {
	Copies     uint64
	USD        float64 // sum over the copies, as reports count it
	CampaignID string
}

// ConversionEventsByID reads one organization's conversion events of a
// status with event_at in [from, to), grouped by event_id (” collects the
// rows that predate event ids).
func (s *EventStore) ConversionEventsByID(ctx context.Context, organizationID, status string, from, to time.Time) (map[string]StoredEvent, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT event_id, count(), sum(usd_value * has_usd_value), any(campaign_id)
		FROM conversion_events
		WHERE organization_id = ? AND type = ? AND event_at >= ? AND event_at < ?
		GROUP BY event_id`, organizationID, status, from, to)
	if err != nil {
		return nil, fmt.Errorf("chstore: reading conversion events by id: %w", err)
	}
	defer rows.Close()
	out := map[string]StoredEvent{}
	for rows.Next() {
		var id string
		var e StoredEvent
		if err := rows.Scan(&id, &e.Copies, &e.USD, &e.CampaignID); err != nil {
			return nil, fmt.Errorf("chstore: scanning conversion events by id: %w", err)
		}
		out[id] = e
	}
	return out, rows.Err()
}

// DeleteConversionEvents removes every copy of the given events from
// conversion_events and from the ltv_events rows they produced, waiting
// until the deletes are applied.
func (s *EventStore) DeleteConversionEvents(ctx context.Context, organizationID string, eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}
	ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
	for _, table := range []string{"conversion_events", "ltv_events"} {
		if err := s.conn.Exec(ctx, `ALTER TABLE `+table+` DELETE WHERE organization_id = ? AND event_id IN ?`,
			organizationID, eventIDs); err != nil {
			return fmt.Errorf("chstore: deleting repaired events from %s: %w", table, err)
		}
	}
	return nil
}

// DailyBucket is one row key of conversion_events_daily_campaign.
type DailyBucket struct {
	CampaignID, Type string
	Day              time.Time // UTC date
}

// RebuildDailyConversionBuckets recomputes the given rows of
// conversion_events_daily_campaign from conversion_events — after a repair
// deleted or re-inserted the events behind them. The aggregate is a
// SummingMergeTree fed by a view on insert, so a delete in the source never
// reaches it on its own.
//
// Not atomic with live traffic: a conversion of the same campaign and day
// inserted between the delete and the re-insert is counted twice. Repairs
// only touch days at least the reconciliation's settle time old, where new
// conversions are rare; the next reconciliation run reports it if it
// happens.
func (s *EventStore) RebuildDailyConversionBuckets(ctx context.Context, organizationID string, buckets []DailyBucket) error {
	sync := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2}))
	// insert_deduplicate=0: with the deduplication window on (schema/012),
	// a recomputed block byte-identical to an earlier one would be dropped
	// right after its rows were deleted.
	noDedup := clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"insert_deduplicate": 0}))
	for _, b := range buckets {
		day := b.Day.Format(time.DateOnly)
		if err := s.conn.Exec(sync, `ALTER TABLE conversion_events_daily_campaign DELETE
			WHERE organization_id = ? AND campaign_id = ? AND type = ? AND day = ?`,
			organizationID, b.CampaignID, b.Type, day); err != nil {
			return fmt.Errorf("chstore: clearing daily bucket: %w", err)
		}
		if err := s.conn.Exec(noDedup, `INSERT INTO conversion_events_daily_campaign
			SELECT organization_id, campaign_id, type, toDate(event_at) AS day, count() AS event_count,
			       sum(usd_value * has_usd_value) AS revenue_usd
			FROM conversion_events
			WHERE organization_id = ? AND campaign_id = ? AND type = ? AND toDate(event_at) = ?
			GROUP BY organization_id, campaign_id, type, day`,
			organizationID, b.CampaignID, b.Type, day); err != nil {
			return fmt.Errorf("chstore: rebuilding daily bucket: %w", err)
		}
	}
	return nil
}

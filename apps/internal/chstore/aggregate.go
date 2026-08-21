package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// DailyCount is one (day, event type) bucket from events_daily_campaign.
type DailyCount struct {
	Day        time.Time
	Type       event.Type
	EventCount uint64
}

// DailyCampaignCounts reads the events_daily_campaign aggregate
// (schema/002_events_daily_campaign.sql) for one campaign over [from, to].
//
// SUM(event_count) is required, not optional: SummingMergeTree merges
// same-key rows only in the background, so a query that trusted "one row
// per (day, type) already holds the final total" would undercount whenever
// a merge hasn't run yet.
func (s *EventStore) DailyCampaignCounts(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]DailyCount, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT day, type, sum(event_count) AS event_count
		FROM events_daily_campaign
		WHERE organization_id = ? AND campaign_id = ? AND day >= ? AND day <= ?
		GROUP BY day, type
		ORDER BY day, type`,
		organizationID, campaignID, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying daily campaign counts: %w", err)
	}
	defer rows.Close()

	var out []DailyCount
	for rows.Next() {
		var (
			day        time.Time
			typ        string
			eventCount uint64
		)
		if err := rows.Scan(&day, &typ, &eventCount); err != nil {
			return nil, fmt.Errorf("chstore: scanning daily campaign count: %w", err)
		}
		out = append(out, DailyCount{Day: day, Type: event.Type(typ), EventCount: eventCount})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading daily campaign counts: %w", err)
	}
	return out, nil
}

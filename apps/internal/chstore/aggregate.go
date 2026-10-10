package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// DailyCount is one (day, event type) bucket from click_events_daily_campaign.
type DailyCount struct {
	Day        time.Time
	Type       event.Type
	EventCount uint64
}

// DailyRevenue is one (day, CPA status) bucket from
// conversion_events_daily_campaign.
type DailyRevenue struct {
	Day        time.Time
	Type       event.Type
	EventCount uint64
	RevenueUSD float64
}

// DailyCampaignCounts reads click_events_daily_campaign
// (schema/006_materialized_views.sql) for one campaign over [from, to].
//
// SUM(event_count) is required, not optional: SummingMergeTree merges
// same-key rows only in the background, so a query that trusted "one row
// per (day, type) already holds the final total" would undercount whenever
// a merge hasn't run yet.
func (s *EventStore) DailyCampaignCounts(ctx context.Context, organizationID, campaignID string, from, to time.Time) ([]DailyCount, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT day, type, sum(event_count) AS event_count
		FROM click_events_daily_campaign
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

// DailyCampaignRevenue is one campaign's conversions and USD revenue per
// UTC day and status over [from, to]. By conversion it reads the rollup
// conversion_events_daily_campaign (same SUM(...) requirement as
// DailyCampaignCounts, for the same reason); by click it reads
// conversion_events, since the rollup only knows the conversion's own day.
func (s *EventStore) DailyCampaignRevenue(ctx context.Context, organizationID, campaignID string, from, to time.Time, basis DateBasis) ([]DailyRevenue, error) {
	query, args := `
		SELECT day, type, sum(event_count) AS event_count, sum(revenue_usd) AS revenue_usd
		FROM conversion_events_daily_campaign
		WHERE organization_id = ? AND campaign_id = ? AND day >= ? AND day <= ?
		GROUP BY day, type
		ORDER BY day, type`, []any{organizationID, campaignID, from.Format("2006-01-02"), to.Format("2006-01-02")}
	if basis == DateByClick {
		start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
		end := time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, time.UTC)
		rangeClause, rangeArgs := conversionRange("", basis, start, end)
		query = `
		SELECT ` + dayExpr(conversionTimeExpr("", basis), "") + `, type, count() AS event_count, sum(usd_value * has_usd_value) AS revenue_usd
		FROM conversion_events
		WHERE organization_id = ? AND campaign_id = ? AND ` + rangeClause + `
		GROUP BY day, type
		ORDER BY day, type`
		args = append([]any{organizationID, campaignID}, rangeArgs...)
	}
	rows, err := s.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying daily campaign revenue: %w", err)
	}
	defer rows.Close()

	var out []DailyRevenue
	for rows.Next() {
		var (
			day        time.Time
			typ        string
			eventCount uint64
			revenueUSD float64
		)
		if err := rows.Scan(&day, &typ, &eventCount, &revenueUSD); err != nil {
			return nil, fmt.Errorf("chstore: scanning daily campaign revenue: %w", err)
		}
		out = append(out, DailyRevenue{Day: day, Type: event.Type(typ), EventCount: eventCount, RevenueUSD: revenueUSD})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading daily campaign revenue: %w", err)
	}
	return out, nil
}

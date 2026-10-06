package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// ConversionEvent is one CPA_* row from conversion_events — the subset of
// columns the browser-facing Conversions list/detail API
// (apps/internal/conversions) needs. Geo/device/attribution columns stay
// unread here since neither view uses them. There is no offer_id: the table
// doesn't carry one (only click_events/tracking_events do) — see
// OffersByClickIDs.
type ConversionEvent struct {
	EventAt      time.Time
	Type         event.Type
	CampaignID   string
	ClickID      string
	NetworkID    string
	FlowID       string
	Revenue      float64
	Currency     string
	USDValue     float64
	HasUSDValue  bool
	NetworkTxnID string
	UTMSource    string
	UTMMedium    string
	UTMCampaign  string
	UTMContent   string
	UTMTerm      string
	Subs         [10]string
}

const conversionEventColumns = `event_at, type, campaign_id, click_id, network_id, flow_id, revenue, currency, usd_value, has_usd_value, network_txn_id,
	utm_source, utm_medium, utm_campaign, utm_content, utm_term,
	sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10`

func scanConversionEvent(r driver.Rows) (ConversionEvent, error) {
	var (
		c      ConversionEvent
		typ    string
		hasUSD uint8
	)
	if err := r.Scan(
		&c.EventAt, &typ, &c.CampaignID, &c.ClickID, &c.NetworkID, &c.FlowID, &c.Revenue, &c.Currency, &c.USDValue, &hasUSD, &c.NetworkTxnID,
		&c.UTMSource, &c.UTMMedium, &c.UTMCampaign, &c.UTMContent, &c.UTMTerm,
		&c.Subs[0], &c.Subs[1], &c.Subs[2], &c.Subs[3], &c.Subs[4], &c.Subs[5], &c.Subs[6], &c.Subs[7], &c.Subs[8], &c.Subs[9],
	); err != nil {
		return ConversionEvent{}, err
	}
	c.Type = event.Type(typ)
	c.HasUSDValue = hasUSD != 0
	return c, nil
}

// ListConversions reads conversion_events for one organization over
// [from, to], newest first — the Conversions list page's data source.
func (s *EventStore) ListConversions(ctx context.Context, organizationID string, from, to time.Time, limit, offset int) ([]ConversionEvent, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT `+conversionEventColumns+`
		FROM conversion_events
		WHERE organization_id = ? AND event_at >= ? AND event_at <= ?
		ORDER BY event_at DESC
		LIMIT ? OFFSET ?`,
		organizationID, from, to, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying conversions: %w", err)
	}
	defer rows.Close()

	var out []ConversionEvent
	for rows.Next() {
		c, err := scanConversionEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("chstore: scanning conversion: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading conversions: %w", err)
	}
	return out, nil
}

// CountConversions is ListConversions' companion for the list page's total
// count (pagination), over the same [from, to] window.
func (s *EventStore) CountConversions(ctx context.Context, organizationID string, from, to time.Time) (int, error) {
	row := s.conn.QueryRow(ctx, `
		SELECT count()
		FROM conversion_events
		WHERE organization_id = ? AND event_at >= ? AND event_at <= ?`,
		organizationID, from, to,
	)
	var total uint64
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("chstore: counting conversions: %w", err)
	}
	return int(total), nil
}

// ConversionsByClickID reads every CPA_* row recorded for one click_id,
// oldest first — a click can carry more than one (HOLD, then ACCEPT, then
// REDEP, ...), and that status history is exactly what the Conversion
// detail page's timeline shows.
func (s *EventStore) ConversionsByClickID(ctx context.Context, organizationID, clickID string) ([]ConversionEvent, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT `+conversionEventColumns+`
		FROM conversion_events
		WHERE organization_id = ? AND click_id = ?
		ORDER BY event_at ASC`,
		organizationID, clickID,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying conversions by click_id: %w", err)
	}
	defer rows.Close()

	var out []ConversionEvent
	for rows.Next() {
		c, err := scanConversionEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("chstore: scanning conversion: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading conversions by click_id: %w", err)
	}
	return out, nil
}

// FunnelEvent is one pre-conversion funnel step (click_events or
// tracking_events) for a click_id — everything the Conversion detail
// page's timeline shows before the conversion itself. CampaignID lets the
// detail page resolve a campaign even for a click_id with funnel events
// but no conversion yet.
type FunnelEvent struct {
	EventAt    time.Time
	Type       event.Type
	CampaignID string
}

// FunnelByClickID reads every SOURCE_CLICK/SOURCE_FILTER (click_events) and
// funnel-stage (tracking_events) row for one click_id, oldest first. Two
// queries rather than one UNION ALL: click_events and tracking_events don't
// share a column list (destination/sticky_applied/config_version are
// click_events-only), and both are cheap point lookups on click_id.
func (s *EventStore) FunnelByClickID(ctx context.Context, organizationID, clickID string) ([]FunnelEvent, error) {
	var out []FunnelEvent
	for _, table := range [2]string{"click_events", "tracking_events"} {
		if err := func() error {
			rows, err := s.conn.Query(ctx, `
				SELECT event_at, type, campaign_id FROM `+table+`
				WHERE organization_id = ? AND click_id = ?
				ORDER BY event_at ASC`,
				organizationID, clickID,
			)
			if err != nil {
				return fmt.Errorf("chstore: querying %s by click_id: %w", table, err)
			}
			defer rows.Close()

			for rows.Next() {
				var (
					eventAt    time.Time
					typ        string
					campaignID string
				)
				if err := rows.Scan(&eventAt, &typ, &campaignID); err != nil {
					return fmt.Errorf("chstore: scanning %s: %w", table, err)
				}
				out = append(out, FunnelEvent{EventAt: eventAt, Type: event.Type(typ), CampaignID: campaignID})
			}
			return rows.Err()
		}(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// StatusStep is one CPA_* status a click_id reached, for the list's
// hold → accept → redep history.
type StatusStep struct {
	ClickID string
	Type    event.Type
	EventAt time.Time
}

// StatusHistoryByClickIDs reads every CPA_* status recorded for the given
// click_ids, oldest first. The caller passes the page's own click_ids (at
// most one list page), so this stays a bounded point lookup on the
// click_id bloom index.
func (s *EventStore) StatusHistoryByClickIDs(ctx context.Context, organizationID string, clickIDs []string) ([]StatusStep, error) {
	if len(clickIDs) == 0 {
		return nil, nil
	}
	rows, err := s.conn.Query(ctx, `
		SELECT click_id, type, event_at
		FROM conversion_events
		WHERE organization_id = ? AND click_id IN ?
		ORDER BY event_at ASC`,
		organizationID, clickIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying status history: %w", err)
	}
	defer rows.Close()

	var out []StatusStep
	for rows.Next() {
		var (
			st  StatusStep
			typ string
		)
		if err := rows.Scan(&st.ClickID, &typ, &st.EventAt); err != nil {
			return nil, fmt.Errorf("chstore: scanning status history: %w", err)
		}
		st.Type = event.Type(typ)
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading status history: %w", err)
	}
	return out, nil
}

// OffersByClickIDs maps click_id → the offer that click was sent to.
// conversion_events has no offer_id, so it is read from click_events (the
// SOURCE_CLICK row carries it; empty for a redirect flow or an inactive
// offer that fell through to a fallback). A click_id with no offer is
// simply absent from the map.
func (s *EventStore) OffersByClickIDs(ctx context.Context, organizationID string, clickIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(clickIDs) == 0 {
		return out, nil
	}
	rows, err := s.conn.Query(ctx, `
		SELECT click_id, argMax(offer_id, event_at)
		FROM click_events
		WHERE organization_id = ? AND click_id IN ? AND offer_id != ''
		GROUP BY click_id`,
		organizationID, clickIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying offers by click_id: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var clickID, offerID string
		if err := rows.Scan(&clickID, &offerID); err != nil {
			return nil, fmt.Errorf("chstore: scanning offers by click_id: %w", err)
		}
		out[clickID] = offerID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading offers by click_id: %w", err)
	}
	return out, nil
}

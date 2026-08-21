package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Deposit is one CPA_HOLD/CPA_ACCEPT/CPA_REDEP row for a click, as
// internal/ltv needs it — everything else in ltv_events is grouping
// metadata already carried on ClickHistory instead.
type Deposit struct {
	EventAt     time.Time
	Type        event.Type
	USDValue    float64
	HasUSDValue bool
}

// ClickHistory is one click's full CPA_HOLD/CPA_ACCEPT/CPA_REDEP history —
// everything internal/ltv needs to derive that click's Reg/FTD anchor
// dates and every deposit to bucket into LTV windows.
type ClickHistory struct {
	ClickID    string
	CampaignID string
	NetworkID  string
	Country    string
	Deposits   []Deposit
}

// LTVFilter narrows which clicks' cohort anchor must fall in [From, To) —
// see ClicksByFTDAnchor/ClicksByRegAnchor for exactly what "anchor" means
// for each. Empty CampaignID/NetworkID/Country match any value — §26.5's
// named filter dimensions minus "source"/"offer", which
// apps/internal/event.Event doesn't carry yet (the same gap documented
// since Phase 24's macro resolver and Phase 26's click_events sort key).
type LTVFilter struct {
	From       time.Time
	To         time.Time
	CampaignID string
	NetworkID  string
	Country    string
}

func (f LTVFilter) whereAndArgs(anchorType event.Type) (string, []any) {
	where := `type = ? AND event_at >= ? AND event_at < ?`
	args := []any{string(anchorType), f.From, f.To}
	if f.CampaignID != "" {
		where += ` AND campaign_id = ?`
		args = append(args, f.CampaignID)
	}
	if f.NetworkID != "" {
		where += ` AND network_id = ?`
		args = append(args, f.NetworkID)
	}
	if f.Country != "" {
		where += ` AND country = ?`
		args = append(args, f.Country)
	}
	return where, args
}

// ClicksByFTDAnchor returns the full deposit history of every click whose
// CPA_ACCEPT (its first deposit — the FTD) falls in [filter.From,
// filter.To). "Full history" includes CPA_REDEP rows arbitrarily far past
// filter.To: a cohort formed in week 1 still needs its week 13
// redeposits to compute ltv_d31_90, so the history fetch is intentionally
// unbounded on the end date.
//
// The anchor query doesn't need MIN()/GROUP BY to find each click's
// "first" CPA_ACCEPT: the dedup key (CLAUDE.md #3) already guarantees at
// most one CPA_ACCEPT row exists per click_id (event_ref is always "" for
// non-REDEP statuses, so a second ACCEPT collides with the first and is
// dropped as a duplicate before it ever reaches conversion_events/
// ltv_events) — the same invariant applies to CPA_HOLD for
// ClicksByRegAnchor. One row per click IS the first occurrence, always.
func (s *EventStore) ClicksByFTDAnchor(ctx context.Context, organizationID string, filter LTVFilter) ([]ClickHistory, error) {
	return s.clicksByAnchor(ctx, organizationID, filter, event.CpaAccept)
}

// ClicksByRegAnchor is ClicksByFTDAnchor's Reg-cohort counterpart, anchored
// on each click's CPA_HOLD instead.
func (s *EventStore) ClicksByRegAnchor(ctx context.Context, organizationID string, filter LTVFilter) ([]ClickHistory, error) {
	return s.clicksByAnchor(ctx, organizationID, filter, event.CpaHold)
}

func (s *EventStore) clicksByAnchor(ctx context.Context, organizationID string, filter LTVFilter, anchorType event.Type) ([]ClickHistory, error) {
	where, args := filter.whereAndArgs(anchorType)
	anchorRows, err := s.conn.Query(ctx, `
		SELECT click_id FROM ltv_events
		WHERE organization_id = ? AND `+where,
		append([]any{organizationID}, args...)...,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying ltv anchor clicks: %w", err)
	}
	var clickIDs []string
	for anchorRows.Next() {
		var id string
		if err := anchorRows.Scan(&id); err != nil {
			anchorRows.Close()
			return nil, fmt.Errorf("chstore: scanning ltv anchor click id: %w", err)
		}
		clickIDs = append(clickIDs, id)
	}
	anchorErr := anchorRows.Err()
	anchorRows.Close()
	if anchorErr != nil {
		return nil, fmt.Errorf("chstore: reading ltv anchor clicks: %w", anchorErr)
	}
	if len(clickIDs) == 0 {
		return nil, nil
	}

	historyRows, err := s.conn.Query(ctx, `
		SELECT click_id, campaign_id, network_id, country, event_at, type, usd_value, has_usd_value
		FROM ltv_events
		WHERE organization_id = ? AND click_id IN (?)
		ORDER BY click_id, event_at`,
		organizationID, clickIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying ltv click histories: %w", err)
	}
	defer historyRows.Close()

	byClick := make(map[string]*ClickHistory, len(clickIDs))
	var order []string
	for historyRows.Next() {
		var (
			clickID, campaignID, networkID, country, typ string
			eventAt                                      time.Time
			usdValue                                     float64
			hasUSDValue                                  uint8
		)
		if err := historyRows.Scan(&clickID, &campaignID, &networkID, &country, &eventAt, &typ, &usdValue, &hasUSDValue); err != nil {
			return nil, fmt.Errorf("chstore: scanning ltv click history row: %w", err)
		}
		h, ok := byClick[clickID]
		if !ok {
			h = &ClickHistory{ClickID: clickID, CampaignID: campaignID, NetworkID: networkID, Country: country}
			byClick[clickID] = h
			order = append(order, clickID)
		}
		h.Deposits = append(h.Deposits, Deposit{EventAt: eventAt, Type: event.Type(typ), USDValue: usdValue, HasUSDValue: hasUSDValue != 0})
	}
	if err := historyRows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading ltv click histories: %w", err)
	}

	out := make([]ClickHistory, 0, len(order))
	for _, id := range order {
		out = append(out, *byClick[id])
	}
	return out, nil
}

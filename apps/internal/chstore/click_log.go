package chstore

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// ClickLogRow is one row of click_events (schema/001_click_events.sql) — the
// Click Log page's raw per-visit record, IP/geo/device fields included as
// stored. See docs/click-log.md for the retention/PII caveat already
// documented on the schema itself (no TTL, no anonymization).
type ClickLogRow struct {
	EventAt time.Time
	Type    string

	CampaignID    string
	ClickID       string
	StreamSetID   string
	FlowID        string
	Destination   string
	StickyApplied bool
	ConfigVersion int64

	Country        string
	Region         string
	City           string
	Device         string
	Platform       string
	OS             string
	OSVersion      string
	Browser        string
	BrowserVersion string
	Language       string
	IsBot          bool
	IsProxy        bool
	ASN            string
	ConnectionType string
	IP             string
	UserAgent      string
	Referrer       string

	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMContent  string
	UTMTerm     string
	Subs        [10]string

	ExternalClickID string
	FBClickID       string
	TTClickID       string
	FilterReason    string
}

const clickLogColumns = `event_at, type, campaign_id, click_id, stream_set_id, flow_id, destination, sticky_applied, config_version,
	country, region, city, device, platform, os, os_version, browser, browser_version, language, is_bot, is_proxy, asn, connection_type, ip, user_agent, referrer,
	utm_source, utm_medium, utm_campaign, utm_content, utm_term,
	sub1, sub2, sub3, sub4, sub5, sub6, sub7, sub8, sub9, sub10,
	` + SourceClickIDExpr + `, fb_click_id, tt_click_id, filter_reason`

func scanClickLogRow(r driver.Rows) (ClickLogRow, error) {
	var row ClickLogRow
	var sticky, isBot, isProxy uint8
	err := r.Scan(
		&row.EventAt, &row.Type, &row.CampaignID, &row.ClickID, &row.StreamSetID, &row.FlowID, &row.Destination, &sticky, &row.ConfigVersion,
		&row.Country, &row.Region, &row.City, &row.Device, &row.Platform, &row.OS, &row.OSVersion, &row.Browser, &row.BrowserVersion, &row.Language, &isBot, &isProxy, &row.ASN, &row.ConnectionType, &row.IP, &row.UserAgent, &row.Referrer,
		&row.UTMSource, &row.UTMMedium, &row.UTMCampaign, &row.UTMContent, &row.UTMTerm,
		&row.Subs[0], &row.Subs[1], &row.Subs[2], &row.Subs[3], &row.Subs[4], &row.Subs[5], &row.Subs[6], &row.Subs[7], &row.Subs[8], &row.Subs[9],
		&row.ExternalClickID, &row.FBClickID, &row.TTClickID, &row.FilterReason,
	)
	row.StickyApplied, row.IsBot, row.IsProxy = sticky != 0, isBot != 0, isProxy != 0
	return row, err
}

// logWindow is the WHERE of the raw-event logs (click log, conversions,
// postback attempts): one organization over [from, to] and, when clickID is
// set, that click only — the lists' "find a click" (exact match).
//
// campaigns, when not nil, keeps only those campaigns' rows — a Buyer's
// read scope (audit TEAM-1b); empty matches nothing. A table without a
// campaign_id (postback_events) is narrowed through the click instead:
// viaClick reads the click's campaign from click_lookup, so a postback for
// an unknown click is outside every scope.
func logWindow(organizationID string, from, to time.Time, clickID string, campaigns []string, viaClick bool) (string, []any) {
	where := `organization_id = ? AND event_at >= ? AND event_at <= ?`
	args := []any{organizationID, from, to}
	if clickID != "" {
		where += ` AND click_id = ?`
		args = append(args, clickID)
	}
	scope, scopeArgs := campaignsClause(campaigns)
	switch {
	case scope == "":
	case viaClick && len(campaigns) > 0:
		where += ` AND click_id IN (SELECT click_id FROM click_lookup WHERE organization_id = ?` + scope + `)`
		args = append(append(args, organizationID), scopeArgs...)
	default:
		where += scope
		args = append(args, scopeArgs...)
	}
	return where, args
}

// ListClickLog reads click_events for one organization over [from, to],
// newest first — the Click Log page's raw per-visit table, mirroring
// ListPostbackAttempts' shape (postback_events.go).
func (s *EventStore) ListClickLog(ctx context.Context, organizationID string, from, to time.Time, clickID string, limit, offset int, campaigns []string) ([]ClickLogRow, error) {
	where, args := logWindow(organizationID, from, to, clickID, campaigns, false)
	rows, err := s.conn.Query(ctx, `
		SELECT `+clickLogColumns+`
		FROM click_events
		WHERE `+where+`
		ORDER BY event_at DESC
		LIMIT ? OFFSET ?`,
		append(args, limit, offset)...,
	)
	if err != nil {
		return nil, fmt.Errorf("chstore: querying click log: %w", err)
	}
	defer rows.Close()

	var out []ClickLogRow
	for rows.Next() {
		row, err := scanClickLogRow(rows)
		if err != nil {
			return nil, fmt.Errorf("chstore: scanning click log row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: reading click log: %w", err)
	}
	return out, nil
}

// CountClickLog is ListClickLog's companion for the list page's total count
// (pagination), over the same [from, to] window.
func (s *EventStore) CountClickLog(ctx context.Context, organizationID string, from, to time.Time, clickID string, campaigns []string) (int, error) {
	where, args := logWindow(organizationID, from, to, clickID, campaigns, false)
	row := s.conn.QueryRow(ctx, `
		SELECT count()
		FROM click_events
		WHERE `+where, args...)
	var total uint64
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("chstore: counting click log rows: %w", err)
	}
	return int(total), nil
}

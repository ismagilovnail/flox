// Package clicklogs is the browser-facing layer over ClickHouse's
// click_events (§48) — the "Click Log" page, a raw per-visit table
// (SOURCE_CLICK/SOURCE_FILTER) alongside the aggregate-only reporting
// apps/internal/analytics already exposes. Read-only, same shape as
// apps/internal/postbacklogs: no writes, no replay — a click_events row
// isn't a retriable action the way a postback attempt is.
package clicklogs

import "time"

// ClickLog is one click_events row for the Click Log page. OrganizationID
// and ConfigVersion are dropped (the UI never renders either) — everything
// else is the row as stored, IP/UA/geo included per §48's schema; see
// docs/click-log.md for the pre-existing no-TTL/no-anonymization caveat
// this page surfaces rather than changes.
type ClickLog struct {
	EventAt time.Time `json:"eventAt"`
	Type    string    `json:"type"` // SOURCE_CLICK | SOURCE_FILTER

	CampaignID    string `json:"campaignId"`
	ClickID       string `json:"clickId"`
	StreamSetID   string `json:"streamSetId,omitempty"`
	FlowID        string `json:"flowId,omitempty"`
	Destination   string `json:"destination,omitempty"`
	StickyApplied bool   `json:"stickyApplied,omitempty"`

	Country        string `json:"country,omitempty"`
	Region         string `json:"region,omitempty"`
	City           string `json:"city,omitempty"`
	Device         string `json:"device,omitempty"`
	Platform       string `json:"platform,omitempty"`
	OS             string `json:"os,omitempty"`
	OSVersion      string `json:"osVersion,omitempty"`
	Browser        string `json:"browser,omitempty"`
	BrowserVersion string `json:"browserVersion,omitempty"`
	Language       string `json:"language,omitempty"`
	IsBot          bool   `json:"isBot,omitempty"`
	IsProxy        bool   `json:"isProxy,omitempty"`
	ASN            string `json:"asn,omitempty"`
	ConnectionType string `json:"connectionType,omitempty"`
	IP             string `json:"ip,omitempty"`
	UserAgent      string `json:"userAgent,omitempty"`
	Referrer       string `json:"referrer,omitempty"`

	UTMSource   string `json:"utmSource,omitempty"`
	UTMMedium   string `json:"utmMedium,omitempty"`
	UTMCampaign string `json:"utmCampaign,omitempty"`
	UTMContent  string `json:"utmContent,omitempty"`
	UTMTerm     string `json:"utmTerm,omitempty"`

	Sub1  string `json:"sub1,omitempty"`
	Sub2  string `json:"sub2,omitempty"`
	Sub3  string `json:"sub3,omitempty"`
	Sub4  string `json:"sub4,omitempty"`
	Sub5  string `json:"sub5,omitempty"`
	Sub6  string `json:"sub6,omitempty"`
	Sub7  string `json:"sub7,omitempty"`
	Sub8  string `json:"sub8,omitempty"`
	Sub9  string `json:"sub9,omitempty"`
	Sub10 string `json:"sub10,omitempty"`

	ExternalClickID string `json:"externalClickId,omitempty"`
	FBClickID       string `json:"fbClickId,omitempty"`
	TTClickID       string `json:"ttClickId,omitempty"`
	FilterReason    string `json:"filterReason,omitempty"`
}

type ListResult struct {
	Logs  []ClickLog `json:"logs"`
	Total int        `json:"total"`
}

// Package conversions is the browser-facing read layer over
// conversion_events/click_events/tracking_events (§48) — the "Conversions"
// list + detail/timeline pages. Like apps/internal/analytics, it is
// deliberately narrow: no writes, no dedup, no delivery. The conversion
// engine itself (dedup, status progression, CLAUDE.md #3) is
// apps/internal/conversion (singular) — pure orchestration wired into
// apps/tracker's hot path, untouched by this package.
package conversions

import (
	"time"

	"github.com/ismagilovnail/flox/apps/internal/event"
)

// Conversion is one CPA_* row for the Conversions list page. A click_id
// can appear more than once (HOLD, then ACCEPT, then REDEP, ...) — that is
// real status history, not a bug in the query.
type Conversion struct {
	EventAt    time.Time  `json:"eventAt"`
	Type       event.Type `json:"type"`
	CampaignID string     `json:"campaignId"`
	ClickID    string     `json:"clickId"`
	NetworkID  string     `json:"networkId"`
	// FlowID is empty when the click wasn't routed through a flow; OfferID is
	// empty for a redirect flow or a click that fell through to a fallback.
	FlowID       string  `json:"flowId,omitempty"`
	OfferID      string  `json:"offerId,omitempty"`
	Revenue      float64 `json:"revenue"`
	Currency     string  `json:"currency"`
	USDValue     float64 `json:"usdValue"`
	HasUSDValue  bool    `json:"hasUsdValue"`
	NetworkTxnID string  `json:"networkTxnId,omitempty"`

	UTMSource   string `json:"utmSource,omitempty"`
	UTMMedium   string `json:"utmMedium,omitempty"`
	UTMCampaign string `json:"utmCampaign,omitempty"`
	UTMContent  string `json:"utmContent,omitempty"`
	UTMTerm     string `json:"utmTerm,omitempty"`
	Sub1        string `json:"sub1,omitempty"`
	Sub2        string `json:"sub2,omitempty"`
	Sub3        string `json:"sub3,omitempty"`
	Sub4        string `json:"sub4,omitempty"`
	Sub5        string `json:"sub5,omitempty"`
	Sub6        string `json:"sub6,omitempty"`
	Sub7        string `json:"sub7,omitempty"`
	Sub8        string `json:"sub8,omitempty"`
	Sub9        string `json:"sub9,omitempty"`
	Sub10       string `json:"sub10,omitempty"`

	// History is every CPA_* status this row's click_id reached, oldest
	// first (hold → accept → redep), the same for each row of one click_id.
	History []StatusStep `json:"history"`
}

// StatusStep is one entry of Conversion.History.
type StatusStep struct {
	Type    event.Type `json:"type"`
	EventAt time.Time  `json:"eventAt"`
}

type ListResult struct {
	Conversions []Conversion `json:"conversions"`
	Total       int          `json:"total"`
}

// TimelineEvent is one entry in a click_id's real, variable-length funnel
// — whatever stages actually happened, not every conversion forced into
// the same fixed six-item shape. Conversion-only fields are zero-valued
// when IsConversion is false.
type TimelineEvent struct {
	EventAt      time.Time  `json:"eventAt"`
	Type         event.Type `json:"type"`
	IsConversion bool       `json:"isConversion"`
	Revenue      float64    `json:"revenue,omitempty"`
	Currency     string     `json:"currency,omitempty"`
	USDValue     float64    `json:"usdValue,omitempty"`
	HasUSDValue  bool       `json:"hasUsdValue,omitempty"`
}

// ClickTimeline is the Conversion detail page's whole payload: every
// funnel + conversion event recorded for one click_id, chronological.
type ClickTimeline struct {
	ClickID    string          `json:"clickId"`
	CampaignID string          `json:"campaignId"`
	NetworkID  string          `json:"networkId,omitempty"`
	Events     []TimelineEvent `json:"events"`
}

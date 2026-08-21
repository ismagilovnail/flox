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
	EventAt     time.Time  `json:"eventAt"`
	Type        event.Type `json:"type"`
	CampaignID  string     `json:"campaignId"`
	ClickID     string     `json:"clickId"`
	NetworkID   string     `json:"networkId"`
	Revenue     float64    `json:"revenue"`
	Currency    string     `json:"currency"`
	USDValue    float64    `json:"usdValue"`
	HasUSDValue bool       `json:"hasUsdValue"`
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

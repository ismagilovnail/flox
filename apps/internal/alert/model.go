// Package alert implements alerts (marketer audit 2026-10-06, #10): rules
// an organization sets up — spend over a threshold, ROI below one, an offer
// capped or paused, a certificate about to expire — and the notifications
// the worker writes when one fires. Notifications show in the app's bell;
// a rule can also send them to the organization's Slack integration.
//
// Evaluation runs in the worker (Evaluator), outside every request path.
// Each condition fires once: the notification's dedup key names the rule,
// the thing and the day (or the pause, or the certificate), and the unique
// constraint on it makes a second evaluation — or a second worker — a no-op.
package alert

import "time"

type Kind string

const (
	KindSpendOver   Kind = "spend_over"
	KindROIBelow    Kind = "roi_below"
	KindOfferCapped Kind = "offer_capped"
	KindOfferPaused Kind = "offer_paused"
	KindSSLExpiring Kind = "ssl_expiring"
)

func (k Kind) Valid() bool {
	switch k {
	case KindSpendOver, KindROIBelow, KindOfferCapped, KindOfferPaused, KindSSLExpiring:
		return true
	}
	return false
}

// perCampaign kinds read the campaign report and can be narrowed to one
// campaign.
func (k Kind) perCampaign() bool { return k == KindSpendOver || k == KindROIBelow }

// Rule is one alert an organization has set up. Threshold's unit depends on
// Kind: USD for spend_over, percent for roi_below, days for ssl_expiring;
// unused for the offer kinds. CampaignID nil checks every campaign.
type Rule struct {
	ID          string    `json:"id"`
	Kind        Kind      `json:"kind"`
	CampaignID  *string   `json:"campaignId"`
	Threshold   *float64  `json:"threshold"`
	MinSpend    *float64  `json:"minSpend"`
	NotifySlack bool      `json:"notifySlack"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type RuleInput struct {
	Kind        Kind
	CampaignID  *string
	Threshold   *float64
	MinSpend    *float64
	NotifySlack bool
	Enabled     bool
}

// Notification is one alert that fired. Params carries what the message is
// built from (campaign/offer/domain id and name, the measured value, the
// threshold) — the UI renders it in the viewer's language.
type Notification struct {
	ID        string         `json:"id"`
	RuleID    *string        `json:"ruleId"`
	Kind      Kind           `json:"kind"`
	Params    map[string]any `json:"params"`
	CreatedAt time.Time      `json:"createdAt"`
	Read      bool           `json:"read"`
}

type NotificationList struct {
	Notifications []Notification `json:"notifications"`
	Unread        int            `json:"unread"`
}

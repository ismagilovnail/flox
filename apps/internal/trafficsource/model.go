// Package trafficsource implements the Traffic Source API (§27):
// handler → service → repository, matching CLAUDE.md's Go architecture,
// shaped like internal/campaign — both are team-managed entities a
// campaign references.
package trafficsource

import "time"

type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusArchived:
		return true
	default:
		return false
	}
}

// CostIntegration mirrors the frontend's CostIntegration type
// (lib/api/traffic-sources.ts) and the traffic_sources.cost_integration
// CHECK constraint (00002, widened by 00034 for google_ads) exactly. It
// records *intent* — which cost data origin this source is meant to use
// — independent of internal/cost's actual per-day amounts (a source
// with CostIntegrationManual still needs entries logged through the
// campaign Cost tab; this field doesn't create them).
type CostIntegration string

const (
	CostIntegrationNone        CostIntegration = "none"
	CostIntegrationManual      CostIntegration = "manual"
	CostIntegrationFacebookAds CostIntegration = "facebook_ads"
	CostIntegrationTikTokAds   CostIntegration = "tiktok_ads"
	CostIntegrationGoogleAds   CostIntegration = "google_ads"
)

func (c CostIntegration) Valid() bool {
	switch c {
	case CostIntegrationNone, CostIntegrationManual, CostIntegrationFacebookAds, CostIntegrationTikTokAds, CostIntegrationGoogleAds:
		return true
	default:
		return false
	}
}

type TrafficSource struct {
	ID               string          `json:"id"`
	OrganizationID   string          `json:"organizationId"`
	Name             string          `json:"name"`
	Type             string          `json:"type"`
	TrackingTemplate string          `json:"trackingTemplate"`
	CostIntegration  CostIntegration `json:"costIntegration"`
	Status           Status          `json:"status"`
	// PostbackURL is called, macro-resolved, when a conversion attributed
	// to one of this source's clicks is recorded with a status in
	// PostbackStatuses (audit 2026-10-15, INT-1). TemplateKey is the
	// catalog template it was set up from (apps/web lib/source-templates.ts).
	PostbackURL      string    `json:"postbackUrl"`
	PostbackStatuses []string  `json:"postbackStatuses"`
	TemplateKey      string    `json:"templateKey"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name             string
	Type             string
	TrackingTemplate string
	CostIntegration  CostIntegration
	PostbackURL      string
	// PostbackStatuses empty means CPA_ACCEPT.
	PostbackStatuses []string
	TemplateKey      string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// (nil) from "sent as empty string" — matching campaign.UpdateInput.
type UpdateInput struct {
	Name             *string
	Type             *string
	TrackingTemplate *string
	CostIntegration  *CostIntegration
	Status           *Status
	PostbackURL      *string
	PostbackStatuses *[]string
	TemplateKey      *string
}

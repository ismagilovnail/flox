// Package trafficsource implements the Traffic Source API (§27/Phase 11):
// handler → service → repository, matching CLAUDE.md's Go architecture.
// Phase 27 landed a deliberately read-only List (a campaign form's source
// picker needed one, nothing else did yet); this phase grows it into full
// CRUD — create/edit/pause/activate/archive/duplicate, mirroring
// internal/campaign's own shape closely since the two packages solve the
// same "list of team-managed entities a campaign references" problem.
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
// (lib/mock/traffic-sources.ts) and the traffic_sources.cost_integration
// CHECK constraint (00002) exactly. It records *intent* — which cost data
// origin this source is meant to use — independent of internal/cost's
// actual per-day amounts (a source with CostIntegrationManual still needs
// entries logged through the campaign Cost tab; this field doesn't create
// them).
type CostIntegration string

const (
	CostIntegrationNone        CostIntegration = "none"
	CostIntegrationManual      CostIntegration = "manual"
	CostIntegrationFacebookAds CostIntegration = "facebook_ads"
	CostIntegrationTikTokAds   CostIntegration = "tiktok_ads"
)

func (c CostIntegration) Valid() bool {
	switch c {
	case CostIntegrationNone, CostIntegrationManual, CostIntegrationFacebookAds, CostIntegrationTikTokAds:
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
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

type CreateInput struct {
	Name             string
	Type             string
	TrackingTemplate string
	CostIntegration  CostIntegration
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// (nil) from "sent as empty string" — matching campaign.UpdateInput.
type UpdateInput struct {
	Name             *string
	Type             *string
	TrackingTemplate *string
	CostIntegration  *CostIntegration
	Status           *Status
}

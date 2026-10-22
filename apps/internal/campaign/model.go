// Package campaign implements the Campaign API (§37):
// handler → service → repository, matching CLAUDE.md's Go architecture
// (thin handlers, business logic in service, no logic in the repository
// beyond the SQL itself).
package campaign

import "time"

type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusDraft    Status = "draft"
	StatusArchived Status = "archived"
)

func (s Status) Valid() bool {
	switch s {
	case StatusActive, StatusPaused, StatusDraft, StatusArchived:
		return true
	default:
		return false
	}
}

type Campaign struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organizationId"`
	TrafficSourceID string `json:"trafficSourceId"`
	Name            string `json:"name"`
	Status          Status `json:"status"`
	FallbackURL     string `json:"fallbackUrl"`
	Notes           string `json:"notes"`
	// ExternalCampaignID is the ad platform's own campaign id (Facebook
	// Ads/TikTok Ads) — how a later ad-spend sync (§74/§27-COST) matches
	// which FLOX campaign a given day's platform-reported spend belongs
	// to. Empty for the common case (most campaigns aren't funded by a
	// connected ad account at all, or don't need per-campaign spend
	// synced yet). See docs/ad-account-connections.md.
	ExternalCampaignID string `json:"externalCampaignId"`
	// OwnerUserID is the buyer the campaign belongs to (audit TEAM-1): its
	// creator unless reassigned; nil when nobody owns it (created by an
	// API key, or its owner was deleted). Without campaign.manage_all a
	// member changes only campaigns they own.
	OwnerUserID *string   `json:"ownerUserId"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateInput struct {
	TrafficSourceID    string
	Name               string
	FallbackURL        string
	Notes              string
	ExternalCampaignID string
	// OwnerUserID: the creating member; "" leaves the campaign unowned.
	OwnerUserID string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent" (nil)
// from "sent as empty string" — a real partial update, not a full replace.
type UpdateInput struct {
	Name               *string
	TrafficSourceID    *string
	FallbackURL        *string
	Notes              *string
	Status             *Status
	ExternalCampaignID *string
}

type ListFilter struct {
	Limit  int
	Offset int
	// Only, when not nil, limits the list to these campaigns — a Buyer's
	// own (campaignscope, audit TEAM-1b). Service.List sets it.
	Only []string
}

type ListResult struct {
	Campaigns []Campaign
	Total     int
}

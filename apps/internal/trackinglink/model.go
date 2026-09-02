// Package trackinglink implements the (domain, slug) -> campaign mapping
// CRUD an operator needs to actually get a URL to send traffic to.
// tracking_links (migration 00007) already backs apps/tracker's real hot
// path (apps/internal/routingstore.ResolveTrackingLink queries it on every
// click) — this package is the missing write side, the same relationship
// apps/internal/domain has to the domains table it shares this migration
// with.
package trackinglink

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

// TrackingLink is a (domain, slug) pair pointing at one campaign. Domain
// and URL are joined/derived for display — the row itself only stores
// DomainID; URL is built here (not left to the frontend to reconstruct)
// so there is exactly one place that knows the tracker's own URL shape
// (apps/tracker/handler.go's `GET /t/{trackingID}`, host-matched against
// domains.domain).
type TrackingLink struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	CampaignID     string    `json:"campaignId"`
	DomainID       string    `json:"domainId"`
	Domain         string    `json:"domain"`
	Slug           string    `json:"slug"`
	URL            string    `json:"url"`
	Status         Status    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type CreateInput struct {
	DomainID string
	Slug     string
}

type UpdateInput struct {
	Status *Status
}

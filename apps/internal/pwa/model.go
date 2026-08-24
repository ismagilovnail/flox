// Package pwa implements the PWA API (§28, §73): handler → service →
// repository, mirroring internal/landing's shape — flat, team-managed,
// no nested children, referenced by flows.pwa_id (migration 00006, not
// populated by any CRUD yet). Simpler than landing: every field is a
// real Web App Manifest value with no internal/external branching and
// nothing server-derived — the manifest preview the frontend renders is
// a direct projection of these columns, not a fiction.
package pwa

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

type Pwa struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organizationId"`
	Name            string `json:"name"`
	ShortName       string `json:"shortName"`
	ThemeColor      string `json:"themeColor"`
	BackgroundColor string `json:"backgroundColor"`
	IconURL         string `json:"iconUrl"`
	StartURL        string `json:"startUrl"`
	// BounceInAppWebview is the §73-required, provider-neutral capability:
	// bounce in-app WebView traffic (FB/IG/TikTok/Telegram) to the
	// external browser so the install prompt can fire. Not vendor
	// moderator detection, which §73 forbids.
	BounceInAppWebview bool      `json:"bounceInAppWebview"`
	Status             Status    `json:"status"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name               string
	ShortName          string
	ThemeColor         string
	BackgroundColor    string
	IconURL            string
	StartURL           string
	BounceInAppWebview bool
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value" — matching landing.UpdateInput/network.UpdateInput.
type UpdateInput struct {
	Name               *string
	ShortName          *string
	ThemeColor         *string
	BackgroundColor    *string
	IconURL            *string
	StartURL           *string
	BounceInAppWebview *bool
	Status             *Status
}

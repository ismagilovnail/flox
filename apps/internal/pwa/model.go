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
	// ThemeAuto: the install page follows the visitor's device theme.
	ThemeAuto bool `json:"themeAuto"`
	// ShowHeader: the install page's top bar (brand mark, title, search).
	ShowHeader bool `json:"showHeader"`
	// ShowBottomNav: the bottom bar with the menu buttons.
	ShowBottomNav bool `json:"showBottomNav"`
	// PushEnabled: the installed app asks its visitor for notifications
	// on launch (docs/push.md).
	PushEnabled bool `json:"pushEnabled"`
	// HeaderTitle/HeaderLogoURL brand the top bar; empty = default "Apps" and mark.
	HeaderTitle   string `json:"headerTitle"`
	HeaderLogoURL string `json:"headerLogoUrl"`
	IconURL       string `json:"iconUrl"`
	StartURL      string `json:"startUrl"`
	// BounceInAppWebview is the §73-required, provider-neutral capability:
	// bounce in-app WebView traffic (FB/IG/TikTok/Telegram) to the
	// external browser so the install prompt can fire. Not vendor
	// moderator detection, which §73 forbids.
	BounceInAppWebview bool   `json:"bounceInAppWebview"`
	Developer          string `json:"developer"`
	Description        string `json:"description"`
	// Rating/RatingCount/SizeMB are nil together when never entered —
	// distinct from 0, which would render as a fabricated "0 stars"/"0 MB"
	// in the store-listing preview (CLAUDE.md #6's "never compute/display
	// against a fabricated zero" applied to this listing-preview data,
	// not just cost/ROI).
	Rating         *float64 `json:"rating,omitempty"`
	RatingCount    *int     `json:"ratingCount,omitempty"`
	DownloadsLabel string   `json:"downloadsLabel"`
	SizeMB         *float64 `json:"sizeMb,omitempty"`
	// Screenshots is an ordered list of image URLs for the listing
	// gallery — same text[] shape postlanding.Postlanding.Events uses.
	Screenshots []string `json:"screenshots"`
	WhatsNew    string   `json:"whatsNew"`
	// RatingStars5..1 are each independently nullable — a star bucket's
	// percentage that was never entered stays "—" in the review
	// breakdown, not a fabricated 0% (same reasoning as Rating/
	// RatingCount/SizeMB above).
	RatingStars5 *float64 `json:"ratingStars5,omitempty"`
	RatingStars4 *float64 `json:"ratingStars4,omitempty"`
	RatingStars3 *float64 `json:"ratingStars3,omitempty"`
	RatingStars2 *float64 `json:"ratingStars2,omitempty"`
	RatingStars1 *float64 `json:"ratingStars1,omitempty"`
	// Version/AgeRating/Category/InteractiveElements back the listing's
	// "detailed information" sheet. Free text, not enums — the reference
	// shows whatever the operator's real listing says ("1.0.0", "12+",
	// "Casino"). Empty means "never entered": the UI hides that row
	// rather than printing a fabricated value.
	Version             string `json:"version"`
	AgeRating           string `json:"ageRating"`
	Category            string `json:"category"`
	InteractiveElements string `json:"interactiveElements"`
	// ContainsAds/InAppPurchases are what the preview says about ads and
	// paid content — the operator's answer, never a default claim.
	ContainsAds    bool `json:"containsAds"`
	InAppPurchases bool `json:"inAppPurchases"`
	// UpdatedDate is the listing's own "updated on" date, as YYYY-MM-DD.
	// nil means never entered: the preview hides the row rather than
	// showing today.
	UpdatedDate *string   `json:"updatedDate,omitempty"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name                string
	ShortName           string
	ThemeColor          string
	BackgroundColor     string
	ThemeAuto           bool
	ShowHeader          bool
	ShowBottomNav       bool
	PushEnabled         bool
	HeaderTitle         string
	HeaderLogoURL       string
	IconURL             string
	StartURL            string
	BounceInAppWebview  bool
	Developer           string
	Description         string
	Rating              *float64
	RatingCount         *int
	DownloadsLabel      string
	SizeMB              *float64
	Screenshots         []string
	WhatsNew            string
	RatingStars5        *float64
	RatingStars4        *float64
	RatingStars3        *float64
	RatingStars2        *float64
	RatingStars1        *float64
	Version             string
	AgeRating           string
	Category            string
	InteractiveElements string
	ContainsAds         bool
	InAppPurchases      bool
	UpdatedDate         *string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value" — matching landing.UpdateInput/network.UpdateInput.
// Rating/RatingCount/SizeMB/RatingStars5..1 nil means "not sent, leave
// as-is," same as every other field here; there's no way to PATCH one
// of these back to SQL NULL once set, matching every other optional
// field in this codebase (none support an explicit-clear PATCH either).
// Screenshots is *[]string for the same reason postlanding.UpdateInput's
// Events is — nil means "not sent", an empty non-nil slice means "sent,
// clear the list".
type UpdateInput struct {
	Name                *string
	ShortName           *string
	ThemeColor          *string
	BackgroundColor     *string
	ThemeAuto           *bool
	ShowHeader          *bool
	ShowBottomNav       *bool
	PushEnabled         *bool
	HeaderTitle         *string
	HeaderLogoURL       *string
	IconURL             *string
	StartURL            *string
	BounceInAppWebview  *bool
	Developer           *string
	Description         *string
	Rating              *float64
	RatingCount         *int
	DownloadsLabel      *string
	SizeMB              *float64
	Screenshots         *[]string
	WhatsNew            *string
	RatingStars5        *float64
	RatingStars4        *float64
	RatingStars3        *float64
	RatingStars2        *float64
	RatingStars1        *float64
	Version             *string
	AgeRating           *string
	Category            *string
	InteractiveElements *string
	ContainsAds         *bool
	InAppPurchases      *bool
	UpdatedDate         *string
	Status              *Status
}

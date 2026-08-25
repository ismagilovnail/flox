// Package pixel implements the Pixel API (§29): handler → service →
// repository, mirroring internal/postlanding's shape closely — both are
// flat, team-managed entities with a curated §43 event subset and no
// nested children. Unlike Postlanding, a Pixel has no URL: `provider` +
// `pixelId` identify where the conversion is reported, not a page the
// visitor is sent to.
//
// Distinct from a Stream Set's `stream_set_pixels` attachment (migration
// 00008): this package only ever owns the Pixel entity itself (its own
// list/detail page) — which pixels a given Stream Set fires is a separate,
// still-blocked phase (no CRUD wires flows/stream_sets to a pixel_id yet).
package pixel

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

type Provider string

const (
	ProviderFacebook Provider = "facebook"
	ProviderTikTok   Provider = "tiktok"
	ProviderSnapchat Provider = "snapchat"
	ProviderTwitter  Provider = "twitter"
	ProviderGeneric  Provider = "generic"
)

func (p Provider) Valid() bool {
	switch p {
	case ProviderFacebook, ProviderTikTok, ProviderSnapchat, ProviderTwitter, ProviderGeneric:
		return true
	default:
		return false
	}
}

// ValidEventTypes is the curated subset of the full §43 event model a
// conversion pixel plausibly fires on — matches
// apps/web/src/lib/mock/pixels.ts's PIXEL_EVENT_TYPES exactly (the
// frontend source of truth being migrated to lib/api/pixels.ts in this
// same phase). Not the full canonical event enum, same precedent as
// postlanding.ValidEventTypes.
var ValidEventTypes = []string{
	"PWA_INSTALL",
	"CPA_HOLD",
	"CPA_ACCEPT",
	"CPA_REDEP",
	"TG_JOIN",
	"NOTIFICATION_SUBSCRIBE",
}

func isValidEventType(e string) bool {
	for _, v := range ValidEventTypes {
		if e == v {
			return true
		}
	}
	return false
}

type Pixel struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Name           string    `json:"name"`
	Provider       Provider  `json:"provider"`
	PixelID        string    `json:"pixelId"`
	Events         []string  `json:"events"`
	Status         Status    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name     string
	Provider Provider
	PixelID  string
	Events   []string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value" — matching postlanding.UpdateInput.
type UpdateInput struct {
	Name     *string
	Provider *Provider
	PixelID  *string
	Events   *[]string
	Status   *Status
}

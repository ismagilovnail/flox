// Package pixel implements the Pixel API (§29): handler → service →
// repository. A pixel is where conversion events get reported — `provider`
// plus the provider's `pixelId` and access token, or a URL template for the
// generic provider — and which events it reports. Which pixels a click's
// events go to is decided by its Stream Set (stream_set_pixels, migration
// 00008); sending is internal/pixelsend. This package also serves a
// pixel's delivery log (pixel_deliveries), which pixelsend writes.
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
	ProviderGeneric  Provider = "generic"
)

func (p Provider) Valid() bool {
	switch p {
	case ProviderFacebook, ProviderTikTok, ProviderSnapchat, ProviderGeneric:
		return true
	default:
		return false
	}
}

// UsesToken reports whether the provider authenticates with the pixel's
// access token (every one but generic, which calls its URL).
func (p Provider) UsesToken() bool {
	return p != ProviderGeneric
}

// ValidEventTypes is the part of the §43 event model a pixel can report:
// the events FLOX actually produces that an ad platform optimizes for.
var ValidEventTypes = []string{
	"PWA_INSTALL",
	"NOTIFICATION_SUBSCRIBE",
	"TG_START",
	"TG_JOIN",
	"CPA_HOLD",
	"CPA_ACCEPT",
	"CPA_REDEP",
}

// DefaultEventNames is the platform event each FLOX event is reported as
// when the pixel has no override (Pixel.EventNames). Platform standard
// events, so the ad platform can optimize for them; generic sends the FLOX
// name itself.
var DefaultEventNames = map[Provider]map[string]string{
	ProviderFacebook: {
		"PWA_INSTALL":            "CompleteRegistration",
		"NOTIFICATION_SUBSCRIBE": "Subscribe",
		"TG_START":               "Contact",
		"TG_JOIN":                "Subscribe",
		"CPA_HOLD":               "Lead",
		"CPA_ACCEPT":             "Purchase",
		"CPA_REDEP":              "Purchase",
	},
	ProviderTikTok: {
		"PWA_INSTALL":            "CompleteRegistration",
		"NOTIFICATION_SUBSCRIBE": "Subscribe",
		"TG_START":               "Contact",
		"TG_JOIN":                "Subscribe",
		"CPA_HOLD":               "SubmitForm",
		"CPA_ACCEPT":             "CompletePayment",
		"CPA_REDEP":              "CompletePayment",
	},
	ProviderSnapchat: {
		"PWA_INSTALL":            "SIGN_UP",
		"NOTIFICATION_SUBSCRIBE": "SUBSCRIBE",
		"TG_START":               "CUSTOM_EVENT_1",
		"TG_JOIN":                "SUBSCRIBE",
		"CPA_HOLD":               "SIGN_UP",
		"CPA_ACCEPT":             "PURCHASE",
		"CPA_REDEP":              "PURCHASE",
	},
}

// EventName is the name eventType is reported to the platform as.
func EventName(provider Provider, overrides map[string]string, eventType string) string {
	if name := overrides[eventType]; name != "" {
		return name
	}
	if name := DefaultEventNames[provider][eventType]; name != "" {
		return name
	}
	return eventType
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
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	Name           string   `json:"name"`
	Provider       Provider `json:"provider"`
	PixelID        string   `json:"pixelId"`
	Events         []string `json:"events"`
	// The access token itself is never returned: HasAccessToken says one is
	// set and AccessTokenLast4 lets an operator tell which.
	HasAccessToken   bool              `json:"hasAccessToken"`
	AccessTokenLast4 string            `json:"accessTokenLast4"`
	TestEventCode    string            `json:"testEventCode"`
	URL              string            `json:"url"`
	EventNames       map[string]string `json:"eventNames"`
	Status           Status            `json:"status"`
	CreatedAt        time.Time         `json:"createdAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
}

type CreateInput struct {
	Name          string
	Provider      Provider
	PixelID       string
	Events        []string
	AccessToken   string
	TestEventCode string
	URL           string
	EventNames    map[string]string
}

// UpdateInput fields are pointers so PATCH can distinguish "not sent"
// from "sent as zero value". AccessToken "" removes the token.
type UpdateInput struct {
	Name          *string
	Provider      *Provider
	PixelID       *string
	Events        *[]string
	AccessToken   *string
	TestEventCode *string
	URL           *string
	EventNames    *map[string]string
	Status        *Status
}

// Credentials is a pixel as the sender needs it, access token decrypted.
// Never serialized.
type Credentials struct {
	ID            string
	Provider      Provider
	PixelID       string
	AccessToken   string
	TestEventCode string
	URL           string
	EventNames    map[string]string
	Status        Status
}

// Delivery is one row of a pixel's delivery log.
type Delivery struct {
	ID                 string    `json:"id"`
	EventType          string    `json:"eventType"`
	ClickID            string    `json:"clickId"`
	Status             string    `json:"status"`
	AttemptCount       int       `json:"attemptCount"`
	ResponseStatusCode *int      `json:"responseStatusCode"`
	Message            string    `json:"message"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

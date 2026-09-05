// Package integrations implements the real backend for Settings >
// Integrations (§30) — the org-level, non-ad-platform half of it:
// Namecheap, Cloudflare, and Slack. Facebook Ads, TikTok Ads, and Google
// Ads are deliberately out of scope here: those are per-traffic-source
// cost integrations (apps/internal/adaccount), matching trafficsource.
// CostIntegration's own singular design — this package only reads a
// real aggregate summary of them (AdPlatformStatus) for the Settings
// panel to render alongside its own three real cards, it never
// duplicates their connect/disconnect flow.
//
// Connecting a provider is a manual "paste your API credential" form,
// not an OAuth consent flow — the same environment constraint
// apps/internal/adaccount's own package doc already documents (no
// registered OAuth app/public callback URL exists here). Namecheap and
// Cloudflare's adapters are real HTTP clients against each platform's
// actual verify-a-credential endpoint, but this project has no live
// Namecheap/Cloudflare account to exercise them against — verified
// structurally, against a fake HTTP transport in tests, never against
// the real API. Slack's adapter posts to a real incoming-webhook URL,
// which needs no registered app at all and so is real, live-testable
// infrastructure the moment an operator has a webhook URL to give it.
package integrations

import "time"

type Provider string

const (
	ProviderNamecheap  Provider = "namecheap"
	ProviderCloudflare Provider = "cloudflare"
	ProviderSlack      Provider = "slack"
)

func (p Provider) Valid() bool {
	switch p {
	case ProviderNamecheap, ProviderCloudflare, ProviderSlack:
		return true
	default:
		return false
	}
}

// RequiredFields lists this provider's own credential field names, in
// display order — both the service layer's validation and the frontend
// form are built off this same list so the two never drift apart.
func (p Provider) RequiredFields() []string {
	switch p {
	case ProviderNamecheap:
		return []string{"apiUser", "apiKey", "clientIp"}
	case ProviderCloudflare:
		return []string{"apiToken"}
	case ProviderSlack:
		return []string{"webhookUrl"}
	default:
		return nil
	}
}

type Status string

const (
	StatusConnected    Status = "connected"
	StatusError        Status = "error"
	StatusNotConnected Status = "not_connected"
)

// Integration is one org's connection to one credential-based provider —
// the credential fields themselves are never part of this shape (write-
// only from the API's perspective), same "never echo a secret back"
// precedent as apikey.Key/adaccount.Connection.
type Integration struct {
	Provider       Provider   `json:"provider"`
	Status         Status     `json:"status"`
	LastVerifiedAt *time.Time `json:"lastVerifiedAt"`
	LastError      *string    `json:"lastError"`
}

// ConnectInput's Fields is a flat string map rather than a per-provider
// struct — three providers, three completely different credential
// shapes, and every field is validated the same way (present, non-empty,
// within RequiredFields) regardless of provider; a oneof struct would add
// ceremony without adding any real type safety this map doesn't already
// give via RequiredFields.
type ConnectInput struct {
	Provider Provider
	Fields   map[string]string
}

// AdPlatformStatus is the read-only aggregate the Settings panel shows
// for Facebook Ads/TikTok Ads/Google Ads — a count of how many of an
// org's traffic sources configured for that platform actually have a
// real ad_account_connections row, sourced from apps/internal/adaccount,
// never duplicated storage.
type AdPlatformStatus struct {
	Provider       string `json:"provider"`
	ConnectedCount int    `json:"connectedCount"`
	TotalCount     int    `json:"totalCount"`
}

// Summary is the full GET /integrations response.
type Summary struct {
	Integrations []Integration      `json:"integrations"`
	AdPlatforms  []AdPlatformStatus `json:"adPlatforms"`
}

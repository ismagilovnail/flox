// Package adaccount implements ad-network account connections (§74,
// §27-COST): credential storage plus the CostProvider interface and the
// Facebook Ads, TikTok Ads and Google Ads adapters that pull spend through
// those credentials. handler → service → repository, one connection per
// traffic source (as trafficsource.CostIntegration is singular).
//
// Connecting is a "paste your credentials" form, not an OAuth consent
// flow: that needs a registered app with a public callback URL. Facebook
// and TikTok need a long-lived access token; Google Ads access tokens
// expire in about an hour, so the operator pastes a refresh token (minted
// once in Google's OAuth playground) with the developer token and OAuth
// client id/secret. The adapters are tested against fake HTTP servers
// only. See docs/ad-account-connections.md.
package adaccount

import (
	"context"
	"time"
)

// Connection is one traffic source's ad-account credentials. AccessToken
// is deliberately absent from this struct — it's write-only from the
// API's perspective (accepted on Connect, never echoed back in any
// response), same "only ever show a masked preview" precedent as
// api_keys.prefix (migration 00010). TokenPreview is derived, not
// stored: the last 4 characters of the real token, safe to display so an
// operator can recognize which credential is connected without exposing
// it.
type Connection struct {
	ID              string `json:"id"`
	OrganizationID  string `json:"organizationId"`
	TrafficSourceID string `json:"trafficSourceId"`
	AdAccountID     string `json:"adAccountId"`
	TokenPreview    string `json:"tokenPreview"`
	// Timezone is the ad account's own IANA zone, the one its daily spend
	// is cut in — '' until a sync has learned it (tech audit AN4).
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ConnectInput both creates a new connection and replaces an existing
// one (re-connecting overwrites the stored credentials in place) — same
// upsert-by-natural-key precedent as cost.UpsertInput. AdAccountID is
// the ad platform's own account id (a Facebook/TikTok ad account id, or
// a Google Ads customer id); AccessToken is facebook_ads/tiktok_ads
// only; DeveloperToken/ClientID/ClientSecret/RefreshToken are google_ads
// only — Service.Connect validates the right subset is present for
// whichever cost_integration the traffic source actually has.
type ConnectInput struct {
	AdAccountID    string
	AccessToken    string
	DeveloperToken string
	ClientID       string
	ClientSecret   string
	RefreshToken   string
}

// Credentials is the subset of a Connection an actual API call to an ad
// platform needs — deliberately a separate type from Connection, not a
// reuse of it: Connection is the API-response shape (every secret field
// permanently absent, see above); Credentials is only ever constructed
// internally, from a repository read a future sync job calls directly,
// never serialized to JSON.
type Credentials struct {
	AdAccountID    string
	AccessToken    string
	DeveloperToken string
	ClientID       string
	ClientSecret   string
	RefreshToken   string
}

// CostProvider is the §74 extensibility interface the real Facebook Ads/
// TikTok Ads spend adapters implement — one shape so the sync path never
// needs to special-case a specific vendor.
//
// Broken down by the ad platform's OWN campaign id, not an account-wide
// total: cost_entries requires a campaign_id (migration 00009, NOT
// NULL), and a traffic source's connected ad account can fund more than
// one FLOX campaign (campaigns.traffic_source_id is many-to-one) — an
// account-level total would have nothing to attribute itself to. The
// sync matches each record's ExternalCampaignID against
// campaign.Repository.ListByExternalID (migration 00019) to find which
// FLOX campaign(s) it belongs to; a record whose id matches nothing
// simply produces no cost_entries row for that day (CLAUDE.md #6: no
// cost for a slice shows as "—", never a false zero) rather than an
// error — an ad account will always have campaigns FLOX doesn't know
// about (ones an operator hasn't mapped, or genuinely unrelated ones
// sharing the same ad account for other business reasons).
type CostProvider interface {
	// DailySpendByCampaign returns one record per (calendar day, ad-
	// platform campaign) in [from, to] for the given ad account, in the
	// ad platform's own native currency (never pre-converted — currency
	// normalization to USD happens once via fx_rates, §50-FX, at the
	// point a record is written into cost_entries, same as every other
	// cost value in this system).
	DailySpendByCampaign(ctx context.Context, creds Credentials, from, to time.Time) ([]DailyCampaignSpendRecord, error)
}

type DailyCampaignSpendRecord struct {
	// Date is a calendar day in the ad account's own time zone (Timezone)
	// — every platform reports daily spend that way.
	Date               time.Time
	ExternalCampaignID string
	Amount             float64
	Currency           string
	// Timezone is the ad account's IANA zone as the platform reports it,
	// '' when it could not be learned (tech audit AN4).
	Timezone string
}

// ConnectionRef identifies one connected ad account for a trusted, Go-only
// batch caller — the ad-spend sync scheduler (§74/§27-COST) — that must
// iterate every connection across every org on a timer, unlike every other
// method on this package's repository, which is scoped to one orgID because
// it serves one tenant's HTTP request. Never HTTP-reachable.
type ConnectionRef struct {
	OrganizationID  string
	TrafficSourceID string
}

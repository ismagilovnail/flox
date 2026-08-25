// Package adaccount implements ad-network account connections (§74/
// §27-COST): credential storage (Phase A) plus the CostProvider
// interface + real Facebook Ads/TikTok Ads adapters (Phase B) that pull
// spend through those credentials. handler → service → repository, one
// connection per traffic source (matching trafficsource.CostIntegration's
// own singular design).
//
// Connecting an account is a manual "paste your access token + ad
// account id" form, not an OAuth consent flow: a real OAuth flow needs a
// registered app with a public callback URL, and neither exists in this
// environment (confirmed via AskUserQuestion before Phase A started).
// The adapters in this package's facebookads/tiktokads subpackages are
// real HTTP clients against each platform's actual API shape, but this
// project has no live Meta/TikTok app credentials to exercise them
// against — they're verified structurally, against a fake HTTP
// transport in tests, never against the real Graph/Marketing API
// (confirmed via AskUserQuestion before Phase B started). See
// docs/ad-account-connections.md.
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
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organizationId"`
	TrafficSourceID string    `json:"trafficSourceId"`
	AdAccountID     string    `json:"adAccountId"`
	TokenPreview    string    `json:"tokenPreview"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// ConnectInput both creates a new connection and replaces an existing
// one (re-connecting overwrites the stored token/account id in place) —
// same upsert-by-natural-key precedent as cost.UpsertInput.
type ConnectInput struct {
	AdAccountID string
	AccessToken string
}

// Credentials is the subset of a Connection an actual API call to an ad
// platform needs — deliberately a separate type from Connection, not a
// reuse of it: Connection is the API-response shape (AccessToken
// permanently absent, see above); Credentials is only ever constructed
// internally, from a repository read a future sync job calls directly,
// never serialized to JSON.
type Credentials struct {
	AdAccountID string
	AccessToken string
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
	Date               time.Time
	ExternalCampaignID string
	Amount             float64
	Currency           string
}

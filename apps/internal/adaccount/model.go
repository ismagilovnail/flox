// Package adaccount implements ad-network account connections (§74/
// §27-COST): the credential-storage half of Facebook/TikTok ad-spend
// import. handler → service → repository, one connection per traffic
// source (matching trafficsource.CostIntegration's own singular design).
//
// This package deliberately stops at storage + validation — nothing here
// calls Facebook's or TikTok's API yet. A real ad-network sync needs a
// registered OAuth app with a public callback URL to do a live consent
// flow; neither exists in this environment, so the connect flow an
// operator uses is a manual "paste your access token + ad account id"
// form instead (confirmed via AskUserQuestion before this phase started).
// The CostProvider interface below documents the exact shape a later
// phase's real Facebook/TikTok adapters will implement against this
// package's Connection — declared now so this phase's storage shape is
// provably sufficient for that future caller, not guessed at.
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

// CostProvider is the §74 extensibility interface a later phase's real
// Facebook Ads / TikTok Ads spend adapters implement — one shape so
// internal/cost's sync path (also a later phase) never needs to
// special-case a specific vendor. Not called from anywhere in this
// phase; declared now because designing Connection/ConnectInput without
// knowing the exact shape a caller needs would be guessing.
type CostProvider interface {
	// DailySpend returns one record per calendar day in [from, to] for
	// the given ad account, in the ad platform's own native currency
	// (never pre-converted — currency normalization to USD happens once
	// via fx_rates, §50-FX, at the point the record is written into
	// cost_entries, same as every other cost value in this system).
	DailySpend(ctx context.Context, creds Credentials, from, to time.Time) ([]DailySpendRecord, error)
}

type DailySpendRecord struct {
	Date     time.Time
	Amount   float64
	Currency string
}

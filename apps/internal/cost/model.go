// Package cost is campaign spend (§27-COST): one row per (campaign,
// traffic source, day) in Postgres cost_entries, entered by hand or synced
// from ad accounts (internal/costsync), and joined into analytics as
// Spend/Profit/ROI/CPA. At this volume a Postgres GROUP BY answers every
// spend question; the ClickHouse cost_events table (chstore/schema/004) is
// schema only.
package cost

import "time"

// Source identifies where an Entry's amount came from: "manual", or the ad
// network the sync (§74, Service.UpsertFromSync) pulled it from —
// cost_entries' CHECK constraint (migrations 00009, 00034).
type Source string

const (
	SourceManual       Source = "manual"
	SourceFacebookAds  Source = "facebook_ads"
	SourceTikTokAds    Source = "tiktok_ads"
	SourceGoogleAds    Source = "google_ads"
	SourceYandexDirect Source = "yandex_direct"
	SourceVKAds        Source = "vk_ads"
)

func (s Source) Valid() bool {
	switch s {
	case SourceManual, SourceFacebookAds, SourceTikTokAds, SourceGoogleAds, SourceYandexDirect, SourceVKAds:
		return true
	default:
		return false
	}
}

// Entry is one (campaign, traffic source, day) spend record.
// TrafficSourceID is nil for a campaign-wide entry not attributed to one
// source — the same optionality cost_entries' own two partial unique
// indexes (00009) encode.
type Entry struct {
	ID              string
	OrganizationID  string
	CampaignID      string
	TrafficSourceID *string
	EntryDate       time.Time
	// Country is ISO 3166-1 alpha-2, or "" for spend not split by country
	// (migration 00071).
	Country  string
	Amount   float64
	Currency string
	// AmountUSD is nil when no fx_rates row exists yet for
	// (Currency, EntryDate) — CLAUDE.md #6/#7: never invented as 0.
	AmountUSD       *float64
	Source          Source
	CreatedByUserID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UpsertInput both creates a new entry and edits an existing day's amount
// — re-submitting the same (campaign, source, day) updates it in place,
// matching cost_entries' own unique-index-as-identity design (00009's
// comment: "re-entering the same day updates it, it doesn't stack").
// No Source field here deliberately: Service.Upsert (the manual-entry
// write path, the only one an HTTP request can ever reach — upsertRequest
// in handler.go has no source field to decode into this in the first
// place) always writes SourceManual; Service.UpsertFromSync (Go-only,
// called by the sync package, never HTTP-reachable) takes its Source as
// a separate, explicit parameter instead of a struct field precisely so
// there's no single shared "trust whatever the caller put here" field
// that both a public request handler and an internal sync could feed
// differently. Note the dedup key does NOT include Source: a sync-
// written day and a manually-entered day for the same (campaign[,
// traffic source], date) are mutually exclusive — whichever writes last
// wins, intentional, not an oversight.
//
// Country joins the identity: a day can hold a row per country. A day of a
// (campaign, source) is either one total (Country "") or split by country,
// never both — Upsert refuses the mix with ErrSplitConflict, so a day's
// rows always add up to its spend.
type UpsertInput struct {
	TrafficSourceID *string
	EntryDate       time.Time
	Country         string
	Amount          float64
	Currency        string
}

type ListFilter struct {
	From, To time.Time
}

// DailySpend is one day's total spend across every entry for a campaign
// in the requested range — the shape the Overview stat cards and chart
// consume, mirroring chstore.DailyRevenue. AllConverted separates "how
// much" from "is that number complete": false when at least one entry
// that day has no USD value on file yet, so a caller can't silently treat
// a partially-converted sum as the whole truth.
type DailySpend struct {
	Day          time.Time
	AmountUSD    float64
	AllConverted bool
}

// CampaignSpend is one campaign's total spend across a date range, every
// traffic source summed together — the shape the report builder and
// Overview dashboard need (cost per campaign, not per campaign per
// day) rather than DailySpend's daily granularity.
type CampaignSpend struct {
	CampaignID   string
	AmountUSD    float64
	AllConverted bool
}

// CampaignDaySpend / SourceDaySpend are the per-day versions of
// CampaignSpend / SourceSpend, for report rows grouped by day.
type CampaignDaySpend struct {
	CampaignID string
	Day        time.Time
	AmountUSD  float64
}

type SourceDaySpend struct {
	TrafficSourceID string
	Day             time.Time
	AmountUSD       float64
}

// SourceSpend is one traffic source's total spend across a date range: the
// spend of every campaign currently on that source, summed. The report
// builder's `source` dimension is the traffic source of the click's
// campaign, so cost is attributed the same way (via the campaign), not via
// cost_entries.traffic_source_id, which is an optional per-entry tag.
// CountryDaySpend is one (day, country) of spend. Country "" is spend
// not split by country (every synced entry, and manual day totals).
type CountryDaySpend struct {
	Day       time.Time
	Country   string
	AmountUSD float64
}

// CountryScope narrows SpendByCountry to the report's own filters: a
// campaign and/or the traffic source campaigns are on. "" does not narrow.
type CountryScope struct {
	CampaignID      string
	TrafficSourceID string
}

type SourceSpend struct {
	TrafficSourceID string
	AmountUSD       float64
	AllConverted    bool
}

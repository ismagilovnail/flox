// Package cost is §27-COST's manual cost entry MVP: one row per
// (campaign, traffic source, day), joined into campaign-detail analytics
// as Spend/Profit/ROI/CPA. Postgres cost_entries (migration 00009/00017)
// is the only store this phase reads or writes — the ClickHouse
// cost_events table (chstore/schema/004) stays schema-only, same as it
// has since Phase 26: at manual-entry volume a Postgres GROUP BY answers
// "this campaign's spend by day" directly, and nothing in this phase
// needs the cross-dimension (country, network) breakdown ClickHouse's
// table exists for. The sync pipeline documented in that schema file's
// comment becomes worth building once FB/TikTok ad-spend import (§74)
// actually produces ClickHouse-scale volume — not before.
package cost

import "time"

// Source identifies where an Entry's amount came from — cost_entries'
// own CHECK constraint (migration 00009) has always allowed
// facebook_ads/tiktok_ads alongside manual, but nothing in this package
// wrote or read anything but "manual" until the ad-spend sync (§74,
// Phase B) started calling Service.UpsertFromSync.
type Source string

const (
	SourceManual      Source = "manual"
	SourceFacebookAds Source = "facebook_ads"
	SourceTikTokAds   Source = "tiktok_ads"
)

func (s Source) Valid() bool {
	switch s {
	case SourceManual, SourceFacebookAds, SourceTikTokAds:
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
	Amount          float64
	Currency        string
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
type UpsertInput struct {
	TrafficSourceID *string
	EntryDate       time.Time
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

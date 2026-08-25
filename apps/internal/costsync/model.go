// Package costsync is the ad-spend sync orchestrator (§74/§27-COST,
// Phase B): pulls a traffic source's connected ad account's daily
// campaign spend through an adaccount.CostProvider adapter, matches
// each record to FLOX campaign(s) via campaign.Repository.ListByExternalID
// (migration 00019), and writes matches into cost_entries via
// cost.Service.UpsertFromSync. Triggered manually for now ("Sync now" in
// the UI, POST /traffic-sources/{id}/connection/sync) — no scheduler
// exists in this environment to run it on a cron, and none is needed to
// prove the sync itself is correct.
package costsync

import "github.com/ismagilovnail/flox/apps/internal/adaccount"

// Providers maps a traffic source's cost_integration value to the
// adaccount.CostProvider that knows how to pull spend for it — the
// sync's one vendor-specific wiring point (§74/CLAUDE.md invariant #11:
// "providers behind interfaces, no vendor lock-in in core logic").
type Providers struct {
	FacebookAds adaccount.CostProvider
	TikTokAds   adaccount.CostProvider
}

// Result summarizes one sync run for the "Sync now" response — a count,
// not a per-record echo, since a real ad account can report hundreds of
// campaign-days in one run. UnmatchedExternalCampaignIDs is capped at
// maxUnmatchedListed so a very fragmented ad account (many campaigns
// nobody has mapped yet) can't bloat the response; RecordsFetched -
// EntriesWritten still tells the operator the true unmatched count even
// once the list itself is truncated.
type Result struct {
	RecordsFetched                int      `json:"recordsFetched"`
	EntriesWritten                int      `json:"entriesWritten"`
	UnmatchedExternalCampaignIDs  []string `json:"unmatchedExternalCampaignIds"`
	UnmatchedExternalCampaignsMax bool     `json:"unmatchedExternalCampaignIdsTruncated"`
}

const maxUnmatchedListed = 20

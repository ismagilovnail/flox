# Cost Ingestion (Phase 27-COST, §27-COST)

"Profit / ROI / ROAS are meaningless without spend. Do NOT leave cost as an
afterthought." Manual cost entry and CSV import per campaign/source/day/
country, implemented in
`apps/internal/cost`, backed by Postgres `cost_entries` (migrations
00009/00017), joined into the campaign-detail Overview as real Spend/
Profit/ROI/CPA — dropped entirely in Phase 27 pending exactly this work.

## Architecture: Postgres end to end, not ClickHouse

Unlike `internal/analytics`/`internal/ltv`, this package never touches
ClickHouse. `chstore/schema/004_cost_events.sql` reserved a `cost_events`
table for cost data back in Phase 26, with a comment promising "the sync
pipeline... is Phase 27-COST's job" — this phase deliberately does not
build that pipeline. At manual-entry volume (at most one row per
campaign/source/day), a plain Postgres `GROUP BY entry_date` answers
"this campaign's daily spend" directly and correctly. ClickHouse's own
advantages — columnar scan across billions of rows, cross-dimension
breakdowns — don't apply at today's volume, so a write-through sync into
`cost_events` would be an abstraction with zero readers, exactly what
CLAUDE.md's "don't add features beyond what the task requires" rules out.
`cost_events` stays schema-only, as it was — this
becomes the right call to revisit once FB/TikTok ad-spend import (§74,
"later" per §27-COST) produces the kind of volume ClickHouse exists for.

## `amount_usd`: added in 00017, not 00009

Migration 00009 (`cost_fx.sql`) gave `cost_entries` an original
`amount`/`currency` but never the USD side CLAUDE.md #7 requires ("store
both original and USD value at event time") — 00017 adds a nullable
`amount_usd` column, same shape as `conversion_events.usd_value` (00013):
NULL means "no `fx_rates` row for (currency, entry_date) yet," never 0.
`Service.Upsert` calls the same `FXConverter` interface
`internal/conversion.FXConverter` defines, satisfied structurally by the
existing `conversion.PostgresFX` (reused, not reimplemented) — USD is
special-cased to a 1:1 rate there already, so the common case (manual
entries in USD, the only currency the Cost tab defaults to) always
converts cleanly with no `fx_rates` seeding required. Other currencies
convert once `apps/worker` has synced rates (`docs/fx.md`); an entry
saved before its rate existed is filled in by the next sync's repair.

## `created_by_user_id`: nullable, and why that's honest

`cost_entries.created_by_user_id` was `NOT NULL` from 00009 on, referencing
`users`. There is no auth yet (Phase 28) — no session, and therefore no
real user id to attribute a dev-created entry to. 00017 drops the
constraint rather than inventing a placeholder user row to satisfy it: a
fabricated "system" user would be a fake fact recorded as real data, which
is worse than an honest NULL. Phase 28 should re-tighten this once real
sessions exist to attribute new rows to.

## Upsert, not create-then-edit

`cost_entries`' own identity is its two partial unique indexes from 00009
— `(campaign_id, traffic_source_id, entry_date)` when a source is set,
`(campaign_id, entry_date)` when it isn't — "re-entering the same day
updates it, it doesn't stack," per that migration's own comment.
`Repository.Upsert` mirrors that exactly: two `INSERT ... ON CONFLICT ...
DO UPDATE` statements (one per partial index, since Postgres can't target
both from a single `ON CONFLICT` clause), chosen by whether
`TrafficSourceID` is set. The frontend's Cost tab has one form, not a
separate create/edit flow — submitting the same day again just overwrites
it, which `TestUpsertCreatesThenUpdatesInPlace` proves keeps the row id
stable and the entry count at exactly one.

## `DailyCampaignSpend`: sums, and flags what it can't sum

`bool_and(amount_usd IS NOT NULL)` per day surfaces a day where at least
one entry still has no FX rate on file, rather than letting `SUM()`
silently skip it and understate spend with no visible sign anything was
missing. `TestDailyCampaignSpendFlagsIncompleteFXDay` seeds one converted
and one unconverted entry on the same day and checks both the sum (only
the converted entry counts) and the flag (false).

## `/campaigns/{campaignId}/cost-entries/daily`, not under `/analytics`

Click/revenue analytics (`internal/analytics`) are mounted behind apps/
api's `if ch != nil` guard, since they genuinely need ClickHouse. Spend
never does (see above) — mounting it under `/analytics` would have made a
Postgres-only endpoint's availability accidentally depend on ClickHouse
being up, which is backwards: cost data should if anything be *more*
available than click/revenue analytics, not coupled to the same failure
mode. `GET .../cost-entries/daily` lives on `cost.Handler` itself,
colocated with the CRUD it summarizes, and comes up whenever Postgres
does.

## Frontend: Spend is a real number, Profit/ROI/CPA are "—" without it

CLAUDE.md #6: "no cost for a slice shows ROI as '—', never a false-positive
computed against zero." `CampaignOverview` (campaign-detail-view.tsx)
distinguishes two cases that look similar but aren't: Spend itself is a
direct sum, so it's `$0.00` when the range genuinely has zero cost
entries — that's an honest number, not a ratio. Profit/ROI/CPA are
derived *from* spend, so each renders `"—"` whenever `hasCost` is false
(no entries at all), and CPA additionally renders `"—"` whenever
conversions are zero regardless of `hasCost` — cost-per-acquisition with
zero acquisitions is a division by zero, not a $0.00 acquisition cost.
Verified live in-browser: a fresh campaign showed Spend `$0.00` / Profit
`—` / ROI `—` / CPA `—`; after logging one $150 entry with zero
conversions, Spend `$150.00` / Profit `-$150.00` / ROI `-100.00%` / CPA
still correctly `—` (the division-by-zero case, caught and fixed during
this same verification pass rather than shipped).

## Deliberately deferred

FB/TikTok Ads API spend pull (§74's `CostProvider` interface, OAuth + dev
token flow, scheduled sync) is explicitly "later" in §27-COST's own text —
not started here. `source` isn't exposed as client-settable on the
upsert endpoint either: the server always writes `'manual'`, since
exposing the field with no real writer for `'facebook_ads'`/`'tiktok_ads'`
would present an unfinished feature as available.

## Country and CSV import (gap 6, 2026-10-13)

`cost_entries.country` (00071): ISO 3166-1 alpha-2, or `''` for spend not
split by country — every row before 00071 and every synced row (the ad
platforms' daily campaign spend has no country). The country joins the
identity, so the unique indexes are `(campaign, [source,] day, country)`.

**A day is one total or a split, never both.** For one (campaign, source
key, day), `Repository.Upsert` refuses a country row when a total exists
and a total when country rows exist (`ErrSplitConflict`, 409), under a
transaction advisory lock on the day so two writers cannot both pass the
check. So a day's rows always add up to its spend, and `DailyCampaignSpend`
/ `SpendByCampaign` keep summing them unchanged. The sync hitting a day an
operator split by hand skips it (`Result.entriesSkippedSplit`) instead of
adding the platform total on top.

**Report by country** — see `docs/analytics-report-api.md` ("Cost joins at
campaign, source or country granularity"): any unsplit spend in the period
(or day) leaves countries without cost rather than understating it.

**CSV import** — `POST /campaigns/{campaignId}/cost-entries/import`, body
`{"csv": "..."}` (2 MB, 5000 rows), `campaign.write`. The campaign comes from
the route. Header-named columns in any order: `date`, `amount`, `currency`
required; `country`, `traffic_source_id` optional. An unknown column rejects
the file (a misspelt `country` would otherwise land every row as a total).
`;` files with decimal commas (European spreadsheet exports) and a UTF-8 BOM
are accepted. The import is all or nothing: every line is validated first
(same rules as the form, plus duplicates and total-vs-split inside the file),
then written in one transaction; a refused file answers 422 with errors
keyed `line N` / `file`, which the Cost tab lists. Rows are manual entries
and overwrite the same (source, day, country).

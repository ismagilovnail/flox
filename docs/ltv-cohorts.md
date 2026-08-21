# LTV & Cohort Engine (Phase 26.5, §26.5)

"A primary reason teams pay for a tracker in this vertical. Do not skip."
FTD (first-deposit) and Reg (registration) cohort tables with LTV windows,
implemented in `apps/internal/ltv`, backed by `apps/internal/chstore`'s
`ltv_events` and exposed at `GET /analytics/ltv/ftd-cohorts` and
`GET /analytics/ltv/reg-cohorts` on `apps/api`.

## Architecture: pure Go over a narrow ClickHouse fetch

Like `internal/routing`, `internal/ltv` is pure — no database, no clock of
its own (the caller passes `asOf`). ClickHouse's only job
(`chstore.ClicksByFTDAnchor`/`ClicksByRegAnchor`) is fetching each
qualifying click's raw `CPA_HOLD`/`CPA_ACCEPT`/`CPA_REDEP` history; every
cohort number — window bucketing, completeness, the rate metrics — is
deterministic arithmetic in `ltv.go`, tested directly against fixtures
(`ltv_test.go`, 11 cases). This is what makes §26.5's own acceptance
criterion — "numbers reconcile against fixtures" — provable the same way
`internal/routing`'s conformance fixture proves weighted flow selection,
rather than trusting a ClickHouse `dateDiff` expression buried in a
materialized view that nothing exercises directly.

## `ltv_events`: a materialized view, not a duplicate ledger

`schema/007_ltv_events.sql` is `conversion_events` filtered to exactly the
three statuses cohort math uses (`CPA_HOLD`/`CPA_ACCEPT`/`CPA_REDEP` —
`CPA_DECLINE`/`CPA_TRASH` appear in no §26.5 formula), same MV-fed-table
pattern as Phase 26's aggregates.

## Anchor uniqueness: a consequence of the dedup key, not a new rule

`ClicksByFTDAnchor`/`ClicksByRegAnchor` don't need `MIN()`/`GROUP BY` to
find each click's "first" `CPA_ACCEPT`/`CPA_HOLD` — CLAUDE.md #3's dedup
key guarantees at most one row of either status ever reaches
`conversion_events` (and therefore `ltv_events`) per click_id, since
`event_ref` is always `""` for non-`CPA_REDEP` statuses and a second one
collides with the first as a duplicate before it's ever recorded. One row
per click IS the first occurrence, always — this is the same invariant
`internal/attribution`'s `ClickResolver` (Phase 26) leans on for
`click_events`.

## Window semantics

- **Bucketing is by whole calendar day**, not exact 24-hour multiples —
  `daysBetween` truncates both timestamps to their UTC date before
  subtracting. "Day 0" means "the same calendar day as the anchor," which
  is what §26.5's "day 0 / days 1-7" language means in a report, not an
  exact-hours technicality.
- **`ltv_total` is capped at day 90** by construction: a deposit landing on
  day 91+ relative to its anchor matches no window and is excluded from
  `LTVTotalUSD`, though it still counts toward the uncapped
  `TotalDeposits`/`TotalDepositRevenueUSD` fields (§26.5 gives no window for
  `total_deposits`, so it isn't given one here either).
- **A window's revenue is never gated by `Complete`.** An incomplete window
  still reports whatever partial revenue has accrued so far —
  §26.5's own acceptance criterion ("incomplete windows are visibly marked,
  not shown as zero") is implemented literally: the flag and the number are
  independent fields, and a caller that ignores `Complete` still sees an
  honest (if partial) number, not a zero that looks like "no revenue" when
  the truth is "not fully observed yet."
- **Completeness is evaluated against the cohort's YOUNGEST member**,
  conservatively. A "day" cohort's members mostly share one calendar date,
  but "week" and "month" cohorts span several — a window isn't complete for
  the whole group until it's complete even for the member whose anchor date
  is latest. See `TestConservativeCompletenessAcrossCohortMembers`.

## The rate metrics, and why they're not both attached to both cohort types

§26.5 lists `ftd_to_redep_rate`/`dep_to_redep` (denominator `cpa_accept`)
and `reg_to_ftd_rate` (denominator `cpa_hold`) without saying which cohort
view each belongs to. Since an FTD cohort's own size *is* its `cpa_accept`
count and a Reg cohort's own size *is* its `cpa_hold` count, attaching a
`cpa_accept`-denominated rate to a Reg cohort (or vice versa) would either
silently use the wrong denominator or require carrying a second count
alongside `AnchorCount` for a metric the cohort view doesn't natively
answer. `FTDCohort` carries `FTDToRedepRate`/`DepToRedepRate`; `RegCohort`
carries `RegToFTDRate` (the share of a Reg cohort's own registrants who
ever reached a first deposit, regardless of when). Both cohort types get
`TotalDeposits`/`TotalDepositRevenueUSD` — denominator-free sums that apply
cleanly to either anchor.

## `LifetimeDaysAvg`: averaged over clicks that redeposited, not all of them

§26.5 defines `lifetime_days` as "days from FTD to last redeposit" — under
that definition, a click with no redeposit has no lifetime, not a lifetime
of zero. `HasLifetimeData` is `false` (not `LifetimeDaysAvg: 0`) whenever
no click in the cohort redeposited at all; when some did and some didn't,
the average is over only the clicks that did — averaging in zeros for the
rest would understate the metric it's supposed to measure.

## Deliberately deferred: `source`/`offer` filters, and the frontend

§26.5 names `campaign, source, offer, cpa/network, country` as filter/
breakdown dimensions. `CampaignID`/`NetworkID`/`Country` are implemented;
`source` (traffic source) and `offer` are not — `event.Event` carries
neither `traffic_source_id` nor `offer_id` yet, the same pre-existing gap
`internal/macro`'s `{source}` token (Phase 24) and `click_events`' sort key
(Phase 26) already documented. Adding that propagation is its own piece of
work, not silently approximated here.

No frontend work in this phase either — consistent with every Go-backend
phase since 23 (conversion engine, postback engine, analytics pipeline,
ClickHouse), and there is no existing LTV/Cohorts UI mock to integrate with
in the first place (ROADMAP.md has no such phase before 27).

## Verified end-to-end

A real seed through `chstore.EventStore.InsertBatch` (the same path
production traffic uses) — two old FTDs in one campaign/network/country
(one with two redeposits, one with none) plus a registrant who never
converted and a brand-new FTD five days old — queried through the real
compiled `api` binary:

- The old cohort's `d0`/`d1_7`/`d8_30` windows summed exactly to the
  seeded revenue, all four windows `Complete: true`, `ltvPerAnchorUsd`
  correctly averaged over 2 anchors, `ftdToRedepRate = 0.5` (1 of 2
  redeposited), `depToRedepRate = 1.0` (2 redep events / 2 accepts).
- The five-day-old cohort showed `d0` complete with its revenue, and
  `d1_7`/`d8_30`/`d31_90` all `Complete: false` with `0` revenue — correctly
  distinguishing "not yet observed" from "observed as zero."
- The Reg cohort for the same period showed `anchorCount: 3` (all three
  registrants) and `regToFtdRate = 0.667` (2 of 3 ever reached a deposit).
- A second organization querying the same date range got `{"cohorts":[]}`
  — tenant isolation holds through the full stack, not just at the
  repository layer.

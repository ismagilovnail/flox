# FX rates (§50-FX)

Every payout and spend is stored in its original currency **and** in USD at
the rate in force on the event's date (CLAUDE.md #7). Reports sum the USD
side only; an amount with no USD value is left out — never counted as zero.

Until the 2026-10-06 technical audit (AN1) nothing filled `fx_rates`, so
every non-USD amount had no USD value and dropped out of Revenue, Cost and
ROI.

## Where rates come from

`apps/worker` runs `internal/fx.Syncer` at startup and every 4 hours
(`FX_PROVIDER`, `docs/environment.md`). The provider is behind
`fx.Provider` (CLAUDE.md #11); the default is `internal/fx/ecb`:

- European Central Bank reference rates: free, no key, one fixing per
  TARGET working day around 16:00 CET.
- ECB quotes against EUR; a USD rate is `r_USD / r_C`, and EUR itself is
  `r_USD`. USD is never stored — it converts 1:1.
- About 30 currencies. **RUB (suspended since 2022), UAH, KZT and other
  currencies common in affiliate traffic are not published** — amounts in
  them stay without a USD value until another provider is added.
- The 90-day feed (~100 KB) is used normally; the full history (~7.5 MB)
  only when a first sync must reach further back.

Each run fetches from a week before the newest stored rate (picking up
corrections), or further back if an older event still lacks a USD value,
or 90 days on an empty table. Rates are upserted by `(currency, rate_date)`.

## Which rate an event gets

The latest rate published **on or before** the event's UTC date, at most
`fx.MaxStaleDays` (7) old — the same rule in `conversion.PostgresFX` (new
conversions and cost entries) and in the repair below. For a conversion the
event's date is `event_at`: the network's `event_time` when it sent one
(`docs/conversion.md`), else when the postback arrived. Weekends and
holidays use the last fixing; an event before today's fixing uses
yesterday's. Never a later rate, never today's rate for a past event.

## Repair

After each sync, `fx.Store.RepairUSD` fills `postbacks.usd_value` and
`cost_entries.amount_usd` that are NULL and now have a rate (an event that
arrived before the first sync, or while the provider was down). A decline
that reversed an accept which had no USD value yet (`reversal_usd_value`
NULL, `docs/conversion.md` "Chargebacks") takes the accept's repaired
value, so the two still net to zero.

Costs are read from Postgres only, so ROI is right as soon as the repair
runs. Conversions are summed from their ClickHouse copy
(`conversion_events`, and the `ltv_events` / `conversion_events_daily_campaign`
views built from it), so `RepairUSD` also returns the buckets
(organization, UTC day, status) whose USD values changed, and the worker
rewrites exactly those from Postgres right after
(`reconcile.Reconciler.RepairBuckets` — the reconciliation's own repair,
`docs/analytics-pipeline.md`). That works for any date, not only the
hourly reconciliation's last 3 days (tech audit AN1).

Two gaps, both caught by `make reconcile FROM=… TO=…`:

- if the ClickHouse rewrite fails (ClickHouse down during the sync), the
  next sync has nothing left to repair in Postgres and does not retry it;
  the hourly reconciliation still covers the last 3 days;
- a bucket with more than `reconcile.MaxRepairsPerRun` (1000) conversions
  to rewrite is skipped and logged at error level.

## Reporting currency

§50-FX: "Base reporting currency configurable per organization" —
`organizations.currency` (Settings → Organization). Storage does not
change: everything stays in USD at the event's own rate. Reports convert
on the way out (`fx.Store.ReportingConversion` → `fx.Conversion`):

- each **day's** USD is multiplied by that day's USD → base factor
  (`1 / rate_to_usd`, same "latest on or before, at most 7 days old" rule
  as above), so a past day reads the same however wide the report is;
- revenue is converted per conversion row inside ClickHouse
  (`chstore.ReportQuery.Money`, a `transform()` over the inlined day →
  factor lists, by the conversion's UTC day); spend per cost-entry day in
  `analytics.convertedCost` (period sums are rebuilt from the per-day
  rows); LTV per purchase in `ltv.Service`;
- a day with **no rate** makes the amount unknown — Revenue/Cost (and
  Profit, ROI, ROAS, EPC, CPA…) come out `null` ("—"), never a partial sum
  and never 0 (CLAUDE.md #6/#7). EPC is therefore nullable on the wire now;
- every report, timeseries, breakdown, dashboard and LTV response carries
  `currency`; the web app formats money with `useReportingCurrency()` (the
  same organization setting) and refetches everything when it changes;
- **alert rules stay in USD** (`ReportInput.KeepUSD`): thresholds are
  typed in USD. Custom metrics compute over the converted values, so a
  money formula comes out in the reporting currency.

The field names (`revenueUsd`, `ltvTotalUsd`, …) are kept for compatibility
— read them as "in `currency`".

## Monitoring

- `flox_fx_sync_total{outcome}` — sync runs.
- `flox_fx_latest_rate_age_seconds` — age of the newest rate.
- Alerts `FXRatesStale` (> 4 days) and `FXSyncFailing` (no success in a
  day) in `infra/alerts.yml`.

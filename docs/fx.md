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
conversions and cost entries) and in the repair below. Weekends and
holidays use the last fixing; an event before today's fixing uses
yesterday's. Never a later rate, never today's rate for a past event.

## Repair

After each sync, `fx.Store.RepairUSD` fills `postbacks.usd_value` and
`cost_entries.amount_usd` that are NULL and now have a rate (an event that
arrived before the first sync, or while the provider was down).

Costs are read from Postgres only, so ROI is right as soon as the repair
runs. **The ClickHouse copy of a conversion (`conversion_events`, and the
`ltv_events` / `conversion_events_daily_campaign` views built from it) is
not repaired by this step** — it keeps `has_usd_value = 0` until the
reconciliation sees the bucket's USD sum differ and rewrites the event from
Postgres (`docs/analytics-pipeline.md`, "Reconciliation"). The worker does
that for the last 3 days every hour; for older days run
`make reconcile FROM=… TO=…` once the rates are in.

## Monitoring

- `flox_fx_sync_total{outcome}` — sync runs.
- `flox_fx_latest_rate_age_seconds` — age of the newest rate.
- Alerts `FXRatesStale` (> 4 days) and `FXSyncFailing` (no success in a
  day) in `infra/alerts.yml`.

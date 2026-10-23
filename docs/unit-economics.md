# What a client costs (BIZ-2)

Audit 2026-10-15, item BIZ-2: measure the cost of serving a client, so plan
prices stop being guesses. This page has the measurement, the model built on
it, and what it means for the plans. The prices of hosting are inputs: the
example figures below are placeholders, not quotes.

## How it was measured

`make costbench` (`apps/cmd/costbench`) does the following:

1. Seeds a campaign with `cmd/loadtestseed`.
2. Sends clicks through the real path at a fixed rate with vegeta: tracker →
   NATS → worker → ClickHouse.
3. Measures:
   - each container's CPU from its cgroup, minus the idle rate measured before
     and after the load (the lower of the two, see the caveat below);
   - each container's peak anonymous memory;
   - each ClickHouse table's and column's growth, after `OPTIMIZE … FINAL`,
     since merged parts are what a table holds after a while.
4. Deletes the organization's rows from ClickHouse and the seeded campaign.

The clicks look like paid social traffic:

- 70% Facebook/Instagram with a 174-character `fbclid`, utm tags and 5 subs;
- 30% TikTok with a 116-character `ttclid`;
- 10 in-app and mobile user agents;
- 300,000 public IPs (10% IPv6).

Every click id is new, so nothing compresses better than real traffic would.

```
make costbench RATE=1000 DURATION=15m     # stack from infra/docker-compose.dev.yml
```

To re-measure on staging, pass `-tracker`, `-clickhouse` and `-containers` to
`go run ./cmd/costbench` (the containers must be on the same host, cgroup v2).

## Results (2026-10-23, dev machine)

Ryzen 7 7745HX, 16 threads. The whole stack runs on one host in Docker, the
Go services under air. Three runs at 1,000 clicks/s:

| Run | Clicks | Arrived | p50 / p99 | CPU, s per 1M clicks | Bytes per click |
|---|---:|---:|---|---:|---:|
| 25 min | 1,500,000 | 1,500,000 | 0.15 / 0.33 ms | (baseline spoiled, below) | 826.9 |
| 15 min | 900,000 | 900,000 | 0.15 / 0.30 ms | 299.7 | 828.6 |
| 3 min | 180,000 | 180,000 | 0.15 / 0.31 ms | 292.9 | 827.2 |
| 3 min, after DATA-7 | 180,000 | 180,000 | 0.16 / 0.34 ms | 329.7 | **543.9** |

**CPU per million clicks** (15-minute run):

| Container | CPU s / 1M clicks | Cores at 1,000/s | Peak memory |
|---|---:|---:|---:|
| tracker | 151.6 | 0.152 | 52 MB |
| ClickHouse | 92.4 | 0.092 | 1.2 GB |
| worker | 45.0 | 0.045 | 31 MB |
| NATS | 10.7 | 0.011 | 16 MB |
| Postgres, Redis | ≈ 0 | 0 | — |
| **total** | **≈ 300** | **0.30** | |

A million clicks take about 300 CPU-seconds, or **0.083 vCPU-hours**. One core
carries about 3,300 clicks/s through the whole pipeline. Postgres does nothing
per click: clicks go through NATS.

**Storage per click:** 544 bytes compressed since DATA-7, 827 before it.
Almost all of it is in two tables:

| Table | Before DATA-7 | After |
|---|---:|---:|
| `click_events` | 461 | 320 |
| `click_lookup` | 366 | 224 |
| daily aggregates | ≈ 0 | ≈ 0 (one row per campaign × day, country × day) |

**Before DATA-7, click ids took 572 of the 827 bytes (69%)**, because each
was stored twice in each table:

- when no `external_click_id` was sent, the tracker wrote `fbclid` or `ttclid`
  into `external_click_id` too, repeating `fb_click_id` or `tt_click_id`;
- this happened in both `click_events` and `click_lookup`.

DATA-7 keeps only an `external_click_id` that was actually sent. Readers take
`external_click_id`, else `fbclid`, else `ttclid` (`chstore.SourceClickIDExpr`),
so attribution, postback macros and the click log see the same value as
before. That saved 284 bytes per click (34%); `fbclid` (109 bytes per table)
is now the largest column.

The rest:

- `visitor_key` and `ip_visitor_key`: 33 bytes each;
- `click_id` and `event_id`: 19 bytes each;
- `ip`: 12 bytes per table;
- `user_agent`: 5 bytes per table. That is low only because the test uses ten
  user agents; real traffic has more.

**The data never leaves.** The 90-day TTL on these tables applies to the
`ip`, `user_agent` and `referrer` columns (GDPR-1), not to rows. After 90 days
a click still holds about **509 bytes** (792 before DATA-7). Only `postback_events` has a row TTL
(13 months).

### Caveat: the first run's CPU

In the 25-minute run, the idle baseline started 30 seconds after a trial run,
while ClickHouse and Postgres were still applying its cleanup (DELETE
mutations, OPTIMIZE). The "idle" came out higher than the load, and both read
as 0. Since then costbench measures idle before and after the load and uses
the lower rate; `report.json` keeps the raw CPU and both idle rates. The other
two runs agree within 3%.

## The model

For a client with **C million clicks per month**, after **m months**:

```
storage_GB  ≈ 0.51 × C × m × r          r = copies kept (replica, backup)
cpu_vCPUh   ≈ 0.083 × C                 per month, average
peak_cores  ≈ 0.30 × peak_rps / 1000    what must be provisioned
monthly     = storage_GB × price_GB + cpu_vCPUh × price_vCPUh
            + fixed / clients + support_hours × rate / lifetime_months
```

The plans at their limits. Storage is shown after 12 months, because it keeps
growing:

| Plan | Clicks / month | Price | CPU per month | Stored after 12 months |
|---|---:|---:|---:|---:|
| Free | 0.1M | 0 ₽ | 0.008 vCPU-h | 0.6 GB |
| Start | 1M | 4,900 ₽ | 0.08 vCPU-h | 6.1 GB |
| Pro | 5M | 14,900 ₽ | 0.4 vCPU-h | 31 GB |
| Business | 20M | 39,900 ₽ | 1.7 vCPU-h | 122 GB |

**An example with placeholder prices.** Assume:

- 15 ₽ per GB-month of SSD, with 2 copies;
- 1.1 ₽ per vCPU-hour (800 ₽ per vCPU-month).

| Plan | CPU | Storage at month 12 | Storage at month 24 | Share of price, month 24 |
|---|---:|---:|---:|---:|
| Start | 0.1 ₽ | 184 ₽ | 367 ₽ | 7.5% |
| Pro | 0.5 ₽ | 918 ₽ | 1,836 ₽ | 12% |
| Business | 2 ₽ | 3,672 ₽ | 7,344 ₽ | 18% |

Before DATA-7 the same table read 12%, 19% and 29% at month 24.

Peaks do not change this. A Business client averages 7.7 clicks/s; at ten
times that, the pipeline needs 0.02 cores.

## What it means

- **Per-click compute is negligible.** Even at Business volume it is a few
  rubles a month. What a client costs is the fixed infrastructure, support and
  storage. The fixed infrastructure (ClickHouse needs about 1 GB of RAM before
  the first click, plus Postgres, NATS, Redis, two trackers, backups and
  monitoring) is unknown until OPS-1 picks the hosting. The break-even is
  `fixed / (price − variable)` clients.
- **Storage is the only variable cost that matters, and it has no ceiling.**
  Rows are kept forever, so a client's cost grows every month at the same
  price. Two levers:
  1. **Raw-click retention per plan** (DATA-6, open), for example 3 / 6 / 13 months, with the
     daily aggregates kept forever. Reports that read raw `click_events` (click
     log, unique clicks, LTV, breakdowns not covered by an aggregate) would end
     at the retention date. The storage cost then stops growing after the
     retention period.
  2. **Stop storing click ids twice:** done in DATA-7, −34%.
- **Prices are not limited by cost.** With realistic storage prices the
  variable cost stays well under the price, until accumulated storage
  catches up. Setting the prices needs the market data from BIZ-3 and BIZ-4
  and the fixed costs from OPS-1.

## Not measured

- **Support time for onboarding.** That has to come from real onboardings
  (§H of the audit). In the model it is `support_hours`.
- **Report queries.** Reads are bounded per organization by CH-1 (slots,
  memory, time per plan) but not priced here.
- **Landing views, postbacks, push.** A flow with a landing adds
  `tracking_events` rows (`LAND_VIEW`, `LAND_CLICK`). A conversion adds rows to
  `conversion_events`, `ltv_events`, `postback_events` and to Postgres
  (`postbacks`). Conversions are 1–5% of clicks, so they matter less than
  clicks. Landings could add a large share.
- **A real network and replicas.** Everything ran on loopback, one node each.
  A replicated ClickHouse doubles the storage (`r`) and the insert CPU.

# Conversion Engine (Phase 23, §45)

Turns an inbound postback into a recorded, deduplicated, correctly-attributed
CPA event, or an honestly-logged reason it wasn't. Implemented in
`apps/internal/conversion`, served at `GET/POST /postback/{networkId}` on
`apps/tracker` (not `apps/worker` — see ARCHITECTURE.md).

## Request flow

```
{networkId} → PostgresNetworkLookup → OrganizationID, AcceptDuplicates
                                              │
raw click_id/status/revenue/currency/txn_id/event_time │
                                              ▼
                                    Mapper.MapStatus (per-network
                                    event_mappings table)
                                              │
                                   unmapped → ResultError, logged, stop
                                              ▼
                                    eventRefFor(status, txnID)
                                              │
                                              ▼
                              Store.LastStatus (progression check)
                                              │
                          refuses (§45) → ResultIgnored, logged, stop
                                              ▼
                          attribution.AttributeConversion (§44 — runs
                          regardless of outcome; UNKNOWN/AMBIGUOUS/
                          NO_IDENTIFIER still get recorded, not dropped)
                                              │
                                              ▼
                             FXConverter.ToUSD (never invents a rate)
                                              │
                                              ▼
                          Store.Record (Postgres, ONE transaction):
                            lock (org, click_id) → progression re-check →
                            dedup insert → on success: CPA_* event into
                            event_queue + outgoing delivery
```

### One transaction per conversion (audit 2026-10-06, D1/D3)

`PostgresStore.Record` writes a success, its CPA event (`event_queue`, the
outbox the worker flushes to ClickHouse) and its outgoing postback
(`postback_deliveries`) in one transaction. Before, the event went through
the tracker's in-memory buffer (dropped when full or when Postgres
blipped) and the delivery was best-effort; the ledger said "success", the
reports never saw it, and a retry from the network came back "duplicate".
Now any failure rolls everything back and the network gets a 500.

The progression rule runs twice: a fast pre-check (possibly from Redis)
that skips attribution for obvious replays, and the deciding one inside the
transaction under `pg_advisory_xact_lock(hashtextextended(org:click_id))`.
Without the lock, a CPA_HOLD replay racing a CPA_ACCEPT could pass the check
on both sides and land last. The success row's `created_at` is
`clock_timestamp()`, not the column default `now()` (the transaction's
start): a transaction that began first but took the lock second must still
sort last for `LastStatus`. `TestConcurrentHoldAndAcceptNeverEndOnHold`
fails without either part.

The postback attempt audit log (`AttemptLogger`) stays outside the
transaction — it is a secondary record for the logs screen, and the
Postgres ledger row is the source of truth.

## Chargebacks (§45; audit 2026-10-06, T4/AN3)

"ACCEPT then DECLINE → recorded, revenue reversed." A CPA_DECLINE or
CPA_TRASH recorded while the click stands accepted — its latest decision
among ACCEPT/DECLINE/TRASH is an ACCEPT — reverses **that accept's payout
only** (the FTD); redeposits are separate deposits with their own txn ids,
and a decline carries none to point at one. The check runs in the record
transaction under the click's lock, so the decline and the accept it
reverses are seen consistently.

- Postgres: the decline row keeps `revenue`/`currency` exactly as the
  network sent them, and records what it reversed in `reversal_revenue`,
  `reversal_currency`, `reversal_usd_value` (the accept's own amount, and
  its USD value at the accept's rate, so the two net to zero; migration
  00053). The row's message says "reverses the CPA_ACCEPT payout …".
- ClickHouse: the decline's event carries the reversal negated as its
  revenue — and never the amount the network sent with it, because reports
  now sum declines. A decline that reversed nothing carries 0.
- Booked on the decline's own date: closed days keep what they showed (the
  same principle as never moving back to CPA_HOLD); the decline's day can
  show negative revenue. Conversion counts do not change.
- Reports and LTV net it in (`ltv_events_mv_v3` copies declines; LTV adds
  them to revenue and the window of their date, not to deposit counts).
- A repeated decline is a duplicate (dedup key) and reverses nothing twice.
  DECLINE then ACCEPT records the accept as usual. ACCEPT → DECLINE →
  ACCEPT does **not** restore the revenue: the second ACCEPT has the same
  dedup key `(click_id, CPA_ACCEPT, "")` as the first and is recorded as a
  duplicate (CLAUDE.md #3). Re-approvals after a chargeback stay reversed.

## Revenue must be a sane amount

`revenue` must be a finite number in `[0, conversion.MaxRevenue]` (1e9).
`strconv.ParseFloat` alone accepts `NaN` and `Inf`; one `NaN` payout made
every ClickHouse revenue sum it touched `NaN`, and a value past
`postbacks.revenue numeric(14,4)` failed the insert with a 500 the network
retried forever (audit 2026-10-06, S3). The tracker answers **400** —
never retried — before anything is recorded; `Service.Record` repeats the
check for other callers (postback replay) and logs those as `error`. A
negative amount is refused too: a reversal is a `CPA_DECLINE` status
(§45), not a negative payout.

## When the conversion happened: `event_time` (tech audit T3)

A postback may say when the conversion happened at the network:
`event_time` (or `ts`) — unix seconds or milliseconds, RFC 3339, or
`YYYY-MM-DD HH:MM:SS` in UTC. Without it, the moment the postback arrives
is used, as before. That time is the conversion's `event_at`: the day it
counts on in reports and the reconciliation, the date of its FX rate
(§50-FX), and its time to conversion. `created_at` stays the arrival, and
the progression and dedup order by it, unchanged — a replay dated last
week still cannot move a conversion back to `CPA_HOLD`.

Before this, nightly replays and slow networks put every conversion on
the day its postback arrived, at that day's rate.

Bounds: up to 5 minutes ahead counts as now (a network's clock running
fast); further ahead, or more than 90 days back
(`conversion.MaxEventAge`), is refused with **400** before anything is
recorded — an older event belongs in a reconciliation, not a postback,
and would rewrite long-closed reports. A time before the click is kept:
the negative time to conversion is the clock-skew signal
`docs/attribution.md` keeps on purpose. Note that a late, dated
conversion changes the report of the day it names — that is the point.

Manual replays from the Postback Logs screen are dated at the replay.

## Why OrganizationID comes from the URL, not the body

Same rule as attribution (CLAUDE.md #5, §36-TENANCY), applied to a caller
that isn't our own frontend: a CPA network hits FLOX directly, so there is no
session or `X-Organization-Id` header to trust. `{networkId}` in the postback
URL (`api.floxlink.io/postback/{networkId}`, per the Phase 13 UI's Incoming
Postbacks card) IS the credential — a ULID, unguessable, looked up once by
`PostgresNetworkLookup.ByID` before a `conversion.Postback` is even
constructed. `conversion.Service` has no `NetworkLookup` dependency at all,
on purpose: the scope is resolved once, at the edge, not re-derivable deeper
in the call stack where it would be easier to forget.

## Why the postbacks table holds every attempt, not just successes

Migration 00008 scoped the dedup unique index to `(organization_id,
click_id, status, event_ref) WHERE NOT network_accepts_duplicates`. A naive
reading suggests only accepted conversions can live in the table — a
duplicate/ignored/error row sharing that same key would violate the index
too. Migration 00013 adds `AND result = 'success'` to the index predicate
instead: only 'success' rows compete for uniqueness, so a duplicate/ignored/
error attempt gets its own row (own id, own `created_at`) without ever
conflicting. This is what makes §45's "log every postback... with replay
ability" and "have we already processed this" answerable from the same
table, with one atomic `INSERT ... ON CONFLICT DO NOTHING` as the only
concurrency-control mechanism dedup needs.

## Redis's actual job

Only the STATUS PROGRESSION check (`Store.LastStatus`) is cached
(`RedisStore` in `redis.go`). The dedup insert itself stays on Postgres's
atomic constraint — deliberately not fronted by a Redis pre-check. A
Redis-first dedup check would need to mark "seen" before the durable insert
is confirmed to save the round trip, and that ordering can permanently lose
a conversion if the insert then fails (Redis says duplicate forever; nothing
was ever recorded). `RedisStore.Record` only ever writes to Redis *after*
Postgres confirms a success, so the cache can be stale or absent but can
never cause a false "already seen." REDIS_URL unset or unreachable at
startup is not fatal — the tracker falls back to `PostgresStore` directly,
still fully correct, just without the fast path (see `apps/tracker/main.go`).

## Known gap: repeated non-REDEP statuses share one dedup key

§45 designates CPA_REDEP as the only "repeatable" status; every other status
gets `event_ref = ""` unconditionally, by design (see `eventRefFor` in
`conversion.go`) — including on a legitimate second occurrence.

This collides with the STATUS PROGRESSION rule's own example: "reversals
really are undone (CPA_ACCEPT after CPA_DECLINE)" is explicitly *allowed* by
progression (it's not a return to CPA_HOLD), but if that reinstated
CPA_ACCEPT is a **second, distinct** deposit event rather than a re-send of
the original, it computes the identical dedup key `(click_id, CPA_ACCEPT,
"")` as the first ACCEPT and is recorded as a duplicate — no new event, no
new revenue.

This is not a bug in the implementation; it's what §45's dedup and
progression rules literally compose to. §45 explicitly forbids the obvious
fix ("Do NOT substitute a locally generated sequence number — a fresh number
per delivery makes every re-send look distinct and disables deduplication
entirely"), so this package does not invent one. If a real network's
reinstatement flow needs to be distinguishable from a re-send of the
original ACCEPT, that needs its own spec amendment (a network-supplied
identifier for reversals, the way CPA_REDEP has one) — not a decision this
phase makes unilaterally. See `TestProgressionAllowsEverythingElse` in
`conversion_test.go` for the exact scenario and boundary this leaves in
place.

## Attribution window

Decided in this phase: there isn't one. See docs/attribution.md.

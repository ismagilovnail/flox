# Conversion Engine (Phase 23, §45)

Turns an inbound postback into a recorded, deduplicated, correctly-attributed
CPA event, or an honestly-logged reason it wasn't. Implemented in
`apps/internal/conversion`, served at `GET/POST /postback/{networkId}` on
`apps/tracker` (not `apps/worker` — see ARCHITECTURE.md).

## Request flow

```
{networkId} → PostgresNetworkLookup → OrganizationID, AcceptDuplicates
                                              │
raw click_id/status/revenue/currency/txn_id  │
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
                          Store.Record (Postgres, atomic dedup insert)
                                              │
                        ResultSuccess → EventSink.Enqueue(CPA_* event)
```

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

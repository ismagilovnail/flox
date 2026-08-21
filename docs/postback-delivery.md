# Outgoing Postback Delivery (Phase 24, §46)

FLOX notifying a network's configured `postback_url` of a status change.
Implemented in `apps/internal/postback`, run by `apps/worker`; queued by
`apps/internal/conversion` on every successfully-recorded CPA event.

## Trigger

Every CPA status (`CPA_HOLD`/`CPA_ACCEPT`/`CPA_REDEP`/`CPA_DECLINE`/
`CPA_TRASH`) that `internal/conversion.Service.Record` records as
`ResultSuccess` queues a delivery, provided the owning network has a
non-empty `postback_url`. §46 doesn't specify which statuses should trigger
delivery — the Phase 13 frontend mock's data generator gives
`CPA_DECLINE`/`CPA_TRASH` a distinct `not_configured` delivery status, but
that reads as mock-data flavor rather than a stated rule, and the URL
template's `{status}` token exists specifically so a network can branch on
it. Restricting by status would be inventing policy the spec never states;
all five fire.

Only `ResultSuccess` fires — `ResultDuplicate`/`ResultIgnored`/`ResultError`
never do, because nothing new actually happened for the network to be told
about. This falls out naturally from where the enqueue call sits in
`Service.Record`, not from a separate check.

## Why a separate table, not more `direction='outgoing'` rows in `postbacks`

Migration 00008 gave `postbacks` a `direction` column anticipating this need,
and an early version of this phase's own code comment said outgoing would
land "onto this same table." It doesn't, and here's why: `postbacks.result`
and its unique dedup index are a one-shot ledger — "have we already accepted
this (click_id, status, event_ref)." A delivery is a different shape
entirely: multiple attempts over time, a backoff timer, a status vocabulary
that doesn't fit `result`'s CHECK (`queued`/`processing`/`success`/`failed`/
`retrying`/`dead` vs. `success`/`duplicate`/`error`/`ignored`). Forcing both
into one column would either overload `result` with two incompatible
meanings or make the dedup index carry a `direction` predicate it has no
other reason to need. `postback_deliveries` (migration 00014) is its own
table instead, connected back via `source_postback_id` so every delivery
still traces to the exact accepted conversion that triggered it.

## Why the queue is Postgres, not Redis

STACK has no message broker, and a delivery's state (was this delivered?
how many times has it been tried?) is not cache-appropriate data — Redis is
documented as cache/rate-limit/dedup-cache only, never a system of record.
`ClaimDue` uses the standard Postgres job-queue pattern: `SELECT ... FOR
UPDATE SKIP LOCKED` inside a CTE, joined to an `UPDATE`, so two worker
replicas polling at the same instant can never claim the same row — no
separate locking layer needed.

## Method, timeout, backoff

- **HTTP GET only.** `networks.postback_url` is documented as a macro
  template (`...?click_id={click_id}&status={status}`), consistent with how
  incoming postbacks and every real CPA network's own convention treat
  postback URLs — a query string, not a request body.
- **10s per-attempt timeout** (`attemptTimeout` in `deliverer.go`). A
  network's endpoint hanging must not stall the whole poll loop.
- **Exponential backoff, 8 attempts, ~21 hours total span**
  (`postback.Backoff`/`MaxAttempts`). Not specified by §46; chosen so a
  network's multi-hour outage still gets delivered without retrying forever.
  A dead-lettered delivery is still visible (Phase 13's Postback Logs UI
  already has a "Resend postback" action) — dead means "stopped retrying
  automatically," not "lost."
- Any non-2xx response, or a request that never got a response at all
  (timeout, DNS failure, connection refused, a malformed URL), is treated
  identically: a retryable failure, dead-lettered only at `MaxAttempts`.
  Nothing about the failure *reason* shortens the retry schedule — a
  malformed `postback_url` template is a configuration bug worth surfacing
  (it's logged in full, same as any other outcome), not a reason to skip
  straight to dead-lettering.

## Macro resolution: what's available today, what isn't

`apps/internal/macro` ports the token contract from
`apps/web/src/lib/macros.ts`. Phase 24 resolves `{click_id}`, `{status}`,
`{revenue}`, `{currency}`, `{campaign_id}`, `{country}`, `{device}`, and
`{sub1}`..`{sub10}` — everything `internal/conversion.Service.Record` already
has in hand (from the postback itself, or from the attributed click when
there is one). `{payout}`, `{offer_id}`, and `{source}` are part of the
shared token vocabulary (the frontend's token picker still lists them) but
pass through literally: nothing currently resolves a Flow's destination
offer or a click's traffic source anywhere in the Go codebase, and wiring
that in is out of scope for a queue/worker/retry phase. Campaign/country/
device/subs are similarly only filled in when the conversion is attributed
(`attribution.Outcome.Attributed()`) — an unattributed conversion still
gets delivered (§44's "do not invent attribution" doesn't mean "don't
notify"), just with those tokens left as literal text in the URL.

## Enqueue failures are best-effort, on purpose

`conversion.DeliveryEnqueuer.Enqueue` has no error return. A conversion that
`internal/conversion` already durably recorded must not be reported back to
the network as failed just because queuing its *outgoing* notification hit
a database blip — the incoming postback truly succeeded. A missed enqueue is
recoverable the same way a dead-lettered delivery is: Phase 13's "Resend
postback" action, independent of this request's own success.

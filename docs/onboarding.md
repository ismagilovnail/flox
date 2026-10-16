# First steps (onboarding checklist)

Audit 2026-10-15, UX-3 part 1. A new organization lands on the Overview
with nothing to read; the "First steps" card says what is left before the
first conversion shows up, in the order an operator meets it:

| Step | Done when (all scoped to the organization) | Link |
|---|---|---|
| `source` | a traffic source that is not archived | /traffic-sources/new |
| `network` | a network that is not archived | /networks/new |
| `mapping` | at least one event mapping | /postbacks (Event mapping tab) |
| `offer` | an offer that is not archived | /offers/new |
| `campaign` | an active tracking link of an active campaign | /campaigns/new |
| `click` | a `SOURCE_CLICK` in ClickHouse | /campaigns |
| `conversion` | a `postbacks` row with `result = 'success'` | /networks |
| `cost` | a cost entry or a connected ad account | /campaigns |

- `GET /onboarding` (`apps/internal/onboarding`), behind `analytics.read`
  like the Overview. Nothing is stored: a step is done the moment its data
  exists, and undone if it is removed.
- Without ClickHouse, or when it fails, `click` is `unknown` (shown as not
  checked, never as done); a recorded conversion proves a click anyway.
- The card hides itself when every step is done, and can be hidden by
  hand — per organization, in that browser (`localStorage`
  `flox-first-steps-dismissed:<orgId>`). It refetches on window focus, so
  a step done on another page ticks when the operator comes back.
- Tests: `onboarding_test.go` (order and counts, the click step without
  ClickHouse, the organization scope against Postgres — mutation-checked),
  `first-steps.test.tsx` (next step, empty while loading or all done,
  dismissal).

## Check a postback (UX-3 part 2)

The network page has a "Check a postback" card under the incoming URL: the
operator pastes the URL as the network will call it (the card starts from
the network's own URL with its first mapped status), and FLOX answers what
it would do with it. Nothing is recorded.

- `POST /postback-logs/check {url}` (`apps/internal/postbackcheck`), behind
  the postback logs' read gate (`offer.read`). The network comes from the
  URL's `/postback/{networkId}` path and must be the organization's own;
  another organization's network answers 404 like an unknown one.
- The answer follows the tracker's own order: a wrong or missing secret is
  `httpStatus: 401`, a bad `revenue` or `event_time` is 400 — the same
  parsing (`conversion.ParseParams`, `conversion.ValidSecret`, shared with
  apps/tracker) — and otherwise 200 with the engine's result: success,
  duplicate, ignored or error, the mapped status, the message, the click
  it would be attributed to, the revenue in USD (with "currency assumed"),
  and the outgoing postback it would queue. `notes` flags a host that is
  not `TRACKER_PUBLIC_URL`'s.
- The engine runs once, not twice: `conversion.Service.Check` takes the
  postback through the same steps as `Record` (`Service.run`) and only the
  last differs — `PostgresStore.Preview` runs Record's success write (click
  lock, progression re-check, decline reversal, dedup constraint, outbox) in
  a transaction it always rolls back. No ledger row, CPA event, outgoing
  postback, pixel, attribution retry, attempt log or Redis write; the
  attribution metric is not counted.
- Tests: `conversion/check_test.go` (against Postgres: success, duplicate,
  progression refusal, reversal, unmapped — rows counted before and after
  each; mutation-checked: committing the preview fails it),
  `postbackcheck_test.go` (engine input, other organization = unknown,
  tracker refusals before the engine, URL problems and the host note),
  `postback-check-card.test.tsx`; the cross-tenant reference test covers
  the route.

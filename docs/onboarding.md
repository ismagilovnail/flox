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

Part 2, not built yet: "Check a postback" — a dry run that says what FLOX
would do with a given postback without recording it.

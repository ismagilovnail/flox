# Plans, usage and limits (BIZ-1)

Audit 2026-10-15, item BIZ-1. Code: `apps/internal/billing`, migration
`00094_billing_plans.sql`, CLI `apps/cmd/billing`, UI *Settings › Plan* and
the banner (`components/shell/plan-banner.tsx`).

This phase covers plans, usage counting and soft limits. **There is no payment provider yet.**
The FLOX team assigns plans by hand after the client pays. A provider comes
later, behind an interface (invariant #11).

## Plans

Plans are rows in `plans`, not code, so a price or a limit changes with an
`UPDATE` rather than a deploy. A `NULL` limit means unlimited. `public = false`
is a custom deal: it can be assigned but is not listed.

| Plan | Clicks / month | Campaigns | Domains | Members | Price |
|---|---|---|---|---|---|
| Free | 100,000 | 5 | 1 | 2 | $0 |
| Start | 1,000,000 | 50 | 5 | 5 | $49 |
| Pro | 5,000,000 | ∞ | 20 | 15 | $149 |
| Business | 20,000,000 | ∞ | ∞ | ∞ | $399 |

The numbers are a draft. The click tiers follow competitors' entry plans,
and the prices are placeholders until BIZ-2 measures what a client costs.

The current plan is the open `organization_plans` row (`ended_at IS NULL`;
a partial unique index allows only one). With no row, the organization is on
Free since it was created, so signup writes nothing. The rows are tenant
data: they are deleted with the organization (FK cascade) and included in its
export.

## Usage

**Clicks = `SOURCE_CLICK` + `SOURCE_FILTER` per UTC calendar month.**
Filtered and bot traffic is counted too, because taking it in costs us the
same. Clicks are read from `click_events_daily_campaign`, the per-day rollup
the `click_events` insert fills synchronously. Its `day` is
`toDate(event_at)` of a UTC `DateTime64`, so its day boundary matches the
period's. A month is at most a few thousand summed rows, never a scan of
events.

The other usage numbers are counts:

- campaigns that are not archived;
- every domain;
- memberships that are not suspended. Invites count, because they take a seat
  once accepted.

When ClickHouse is down or the read fails, clicks are **unknown** (`null`,
`level: unknown`). They are never treated as zero, and unknown clicks never
block anything.

## Limits — soft

| Clicks this month | Effect |
|---|---|
| < 80% | none |
| ≥ 80% | `warning`: a banner on every page |
| ≥ 100% | `exceeded`: a red banner |
| ≥ 110% | `blocked`: creating or duplicating a **campaign** or **stream set** answers `402 plan_limit` |

**The tracker is never limited.** Redirects, postbacks and reports keep
working at any usage, so the client's paid traffic is never lost and the hot
path (invariant #9) does not read anything new. This includes Free: a
hard stop for Free was considered and declined.

Count limits refuse the create that would go past them with
`402 plan_limit`. They apply to creating or duplicating a campaign, adding a
domain, and inviting a member. An organization already over a count limit
(for example, after a downgrade) keeps everything it has; only new items are
refused.

Services check limits through `billing.Gate` (`WithQuota` / `SetQuota` on
campaign, streamset, domain and auth). A service built without a gate (tests,
tools, devfixtures) has no limits. The count is read before the insert, not
inside it, so two creates at the same moment can both take the last free
slot. That is acceptable for a soft limit.

## API

- `GET /v1/billing`: plan, `since`, `period {from, to}` (days, inclusive),
  `usage {clicks|null, campaigns, domains, members}`, `level`.
- `GET /v1/billing/plans`: the public plans.

Both are in the public OpenAPI reference (tag *Billing*). Every member can
read them, whatever their role (`rbac_read_coverage_test.go`
`openToEveryMember`), because everyone is held to the limits. The plan cannot
be changed through the API.

## Changing a plan

```
cd apps
DATABASE_URL=… go run ./cmd/billing plans
DATABASE_URL=… go run ./cmd/billing show -org <id>
DATABASE_URL=… go run ./cmd/billing set-plan -org <id> -plan pro -note "invoice 2026-031"
```

`set-plan` runs in a transaction that locks the organization row. It closes
the current plan, opens the new one, and writes a `billing.plan_changed` row
to the organization's audit log with `{from, to, note}` and no actor user.
The new limits apply immediately; the click count does not reset, because
the period is the calendar month.

Dev fixtures put *FLOX Demo* on Business and leave *FLOX Other* on Free, so
both states can be seen.

## Not yet (BIZ-1b and later)

- A payment provider: checkout, webhooks, idempotency, failed payments,
  cancellation.
- Invoices and overage charges.
- A staff admin screen for plans.
- Billing periods anchored to the subscription date instead of the
  calendar month.

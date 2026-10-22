# Plans, usage, limits and payments (BIZ-1)

Audit 2026-10-15, item BIZ-1.

- Code: `apps/internal/billing` (YooKassa adapter in `billing/yookassa`).
- Migrations: `00094_billing_plans.sql` and `00095_billing_payments.sql`.
- CLI: `apps/cmd/billing`.
- UI: *Settings › Plan* and the banner (`components/shell/plan-banner.tsx`).

Plans are prepaid through YooKassa (see [Payments](#payments)) or assigned by
hand with the CLI.

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

## Report limits (CH-1)

An organization's reports share ClickHouse by plan. The limits are in the
plan's row (`plans.ch_*`, migration 00096), so they change without a deploy:

| Plan | Queries at once | `max_threads` | Time | Memory | `priority` |
|---|---|---|---|---|---|
| Free | 2 | 2 | 15 s | 1 GiB | 4 |
| Start | 3 | 4 | 30 s | 2 GiB | 3 |
| Pro | 4 | 8 | 30 s | 4 GiB | 2 |
| Business | 6 | 16 | 30 s | 4 GiB | 1 |

The numbers are a draft, like the prices.

**What enforces them:** `chconn.TenantConn` wraps apps/api's ClickHouse
connection. Every query made with an organization in its context:

- takes one of the organization's slots and holds it until its result is
  read or closed;
- runs with the plan's `max_threads`, `max_execution_time`,
  `max_memory_usage` and `priority` (a lower value runs first when queries
  compete);
- carries `quota_key` = the organization's id.

When all of an organization's slots are taken, a query waits up to 10 s,
then answers `429 reports_busy` with `Retry-After: 5`. The app translates the
message and retries such a query up to 3 times (`shouldRetry`), so a busy
moment rarely shows. Another organization is never held up by it.

Queries with no organization in their context run with the binary's own
limits (30 s / 4 GiB), as before:

- migrations;
- the offer-cap monitor;
- the click count behind `/billing`.

**Plan lookup:** limits are read from Postgres and kept for a minute per
organization, so a plan change applies within a minute. If they cannot be
read, Free's limits apply, so isolation never lifts.

**Settings from code:** code that runs in apps/api and needs query
settings uses `chconn.Settings(ctx, …)`. It does not use
`clickhouse.WithSettings`: the driver's option replaces every earlier
setting and cannot be read back, so the two would drop each other's.

**Per process:** slots are counted per API process, so N replicas allow up
to N× a plan's concurrency. For a server-side cap, see the ClickHouse quota
in docs/deployment.md "Scaling".

The Overview sends two dashboard requests at once (this period and the one
before), plus the onboarding click probe. On Free the third waits for a
fraction of a second.

Metrics: `flox_clickhouse_tenant_waits_total{plan,outcome=waited|busy}`
and `flox_clickhouse_tenant_wait_seconds`.

## API

- `GET /v1/billing`: plan, `since`, `period {from, to}` (days, inclusive),
  `usage {clicks|null, campaigns, domains, members}`, `level`.
- `GET /v1/billing/plans`: the public plans, with `priceRub` (kopecks per
  month, `null` = not sold online).

`GET /v1/billing` also carries `paidUntil`, `graceUntil`, `renew` and `canPay`.

Both are in the public OpenAPI reference (tag *Billing*). The following stay
out of the reference, because they are the app's or YooKassa's:

- `POST /v1/billing/checkout`;
- `GET /v1/billing/payments[/{id}]`;
- the notification URL.

Every member can read `/billing` and `/billing/plans`, whatever their role
(`rbac_read_coverage_test.go` `openToEveryMember`), because everyone is held
to the limits. Payments need `settings.write`, and a checkout needs an Owner.
An API key cannot change the plan.

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

## Payments

YooKassa takes one-stage payments in roubles: `capture: true`, confirmed by
a redirect to YooKassa's page. It sits behind `billing.PaymentProvider`
(invariant #11).

Payments are prepaid: 1, 3 or 12 months at `plans.price_rub × months`, with no
discount for longer terms. There are no automatic charges and no saved cards.

**Checkout.** Only an Owner can start one (`POST /v1/billing/checkout`
`{plan, months}`); the route also needs `settings.write`.

1. A `billing_payments` row is written first, and its id is the
   `Idempotence-Key`, so a retried create cannot charge twice.
2. The YooKassa payment carries:
   - `metadata {payment_id, organization_id}`;
   - `return_url` = `APP_URL/settings?tab=billing&payment=<id>`;
   - a receipt, when `YOOKASSA_RECEIPT=on` (docs/environment.md).
3. The answer's `confirmationUrl` is where the browser goes.

If YooKassa is down, the checkout answers 503 and the row is cancelled.

**The provider's word only.** YooKassa's HTTP notifications are not signed.
`POST /v1/payment-notifications/yookassa` is public. It takes only
`object.id` from the body and handles only `payment.*` events (a `refund.*`
event names a refund and is ignored with 200). It then reads the payment
back through the API (`GET /payments/{id}`) and applies what YooKassa says.

A forged body therefore changes nothing (`TestNotificationBodyIsNotTrusted`).
A payment that isn't ours gets 200, so YooKassa stops retrying. A YooKassa
outage gets 500, so YooKassa retries.

**Applying a payment** (`Repository.ApplyStatus`) happens once, in one
transaction that locks the payment row and the organization. `applied_at` is
set there, so a repeated notification, the worker's read-back and the payer's
return page can all arrive and the plan moves once.

A payment whose amount or currency differs from the checkout is not applied.
The new end of the paid period (`Extend`) is:

- **same plan, still paid:** the months are added to `paid_until`;
- **same plan, in grace, or open-ended:** the months start now;
- **another plan:** the months start now. The unused paid time of the old plan
  is carried over at the two monthly prices: 10 days of 4 900 ₽ become about
  3.3 days of 14 900 ₽. Nothing is refunded.

Each applied payment writes a `billing.payment_applied` audit row.

**Paid period and grace.** `organization_plans.paid_until` holds the end of
the paid period; `NULL` means open-ended (Free, or a plan assigned with the
CLI). The plan stays on for `GracePeriod` (3 days) after `paid_until`.
`apps/worker` (`billing.Runner`, every 10 minutes):

- **Reads back lost notifications:** pending payments 5 minutes to 48 hours
  old are read from YooKassa. In dev without a public notification URL this
  is what applies payments; the payer's return page also reads its own
  payment at once. Pending payments older than 48 hours are cancelled as
  `expired`. If YooKassa later says one succeeded, it still applies, because
  the money was taken.
- **Sends one reminder per period:** from 3 days before `paid_until`, the
  Owners get one "renew" email (`reminded_at`). The UI shows a renew banner
  and the page offers "Renew".
- **Moves lapsed plans to Free:** after `paid_until + 3 days`, the plan is
  closed, a Free row is opened, a `billing.plan_expired` audit row is written
  and the Owners are emailed. Data and tracking are untouched; Free's soft
  limits apply.

**Refunds** are made by hand in the YooKassa dashboard; the plan is then
changed with `apps/cmd/billing set-plan`.

**Dev:** put a YooKassa *test* shop's id and key into your shell or
`infra/.env` (`YOOKASSA_SHOP_ID`, `YOOKASSA_SECRET_KEY`) and recreate `api`
and `worker`. Pay with YooKassa's test cards. The automated tests use a fake
YooKassa server and never need a shop.

## Not yet

- Automatic renewals (saved payment method). YooKassa must enable these for
  the shop.
- Discounts for 3 or 12 months; invoices for legal entities; overage charges.
- A staff admin screen for plans.
- Billing periods anchored to the payment date instead of the calendar month,
  for click usage.

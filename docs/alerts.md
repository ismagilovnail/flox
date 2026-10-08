# Alerts

Marketer audit 2026-10-06, #10. Rules an organization sets up under
**Settings → Alerts**; when one trips, a notification appears in the bell in
the top bar and, if the rule says so, in the organization's Slack channel.

## Rules (`alert_rules`, migration 00057)

| Kind | Threshold | Fires when | Once per |
|---|---|---|---|
| `spend_over` | USD | a campaign's spend today is over the threshold | rule, campaign, day |
| `roi_below` | percent; optional `min_spend` USD | a campaign's ROI today is below the threshold (and spend ≥ `min_spend`) | rule, campaign, day |
| `offer_capped` | — | an offer reached its daily conversion cap (`internal/offercap`) | offer, day |
| `offer_paused` | — | an offer was set to paused | pause (`offers.status_changed_at`) |
| `ssl_expiring` | days, 1–90 | a domain's certificate (`domains.ssl_expires_at`) expires within the days, or has expired | domain, certificate, threshold |

- "Today" is the calendar day in the organization's timezone, the same day
  the Analytics page and the offer cap use.
- Spend and ROI come from the campaign report (`analytics.Service.Report`,
  dimension `campaign`) — the numbers the Analytics page shows. No spend on
  file → neither fires (CLAUDE.md #6: no cost, no ROI). A campaign with spend
  but no events that day is in the report (spend, revenue 0, ROI −100%).
- `campaign_id` narrows spend/ROI rules to one campaign; empty checks every
  campaign separately.
- A pause fires only if it happened after the rule was created and within the
  last 24 h, so switching a rule on does not dig up old pauses.
- Every organization gets `offer_capped`, `offer_paused` and `ssl_expiring`
  (14 days) on creation (trigger `organizations_default_alert_rules`);
  migration 00057 added them to existing organizations.
- Changing rules needs `settings.write`. Reading rules and notifications is
  open to every member.

## Evaluation (`internal/alert`, worker)

`alert.Evaluator.RunLoop` runs every 2 minutes in the worker: it evaluates
every enabled rule of every organization, inserts a notification per
condition with a dedup key (`UNIQUE (organization_id, dedup_key)`, so a
second evaluation or a second worker is a no-op), then delivers pending Slack
notifications. One organization failing is logged and does not stop the
others; a failed offer-cap read skips only the cap rules for that round.

## Notifications (`notifications`, `notification_reads`)

- `GET /notifications` — the latest 50, newest first, each with `read`, and
  the caller's `unread` count. `params` holds ids, names and numbers; the UI
  renders the text in the viewer's language (`features/alerts/notification-text.ts`).
- `POST /notifications/read` — marks everything so far read for the calling
  user only (`notification_reads.read_until`). API keys have no user: they
  see everything as unread and cannot mark read.
- The bell polls once a minute and links each notification to its campaign,
  offer or the Domains page.

## Slack

A rule with `notifySlack` queues its notifications for Slack
(`slack_status = pending`). The worker posts them through the organization's
Slack integration (`integrations.Service.PostSlack`, the same webhook
Settings → Integrations verifies, with the SSRF-safe dialer), in English.
A failed post is retried after 1, 4, 9, 16 minutes and marked `failed` after
5 attempts; an organization without Slack is marked `failed` at once with the
reason. The worker needs `INTEGRATION_ENCRYPTION_KEY` (`docs/environment.md`).

## API

```
GET    /alert-rules            → {rules: Rule[]}
POST   /alert-rules            {kind, campaignId?, threshold?, minSpend?, notifySlack, enabled?}
PUT    /alert-rules/{id}       same body
DELETE /alert-rules/{id}
GET    /notifications          → {notifications: Notification[], unread}
POST   /notifications/read
```

Validation: `spend_over` threshold > 0; `roi_below` threshold in [−100, 100000],
`minSpend` ≥ 0; `ssl_expiring` whole days 1–90; the campaign must belong to the
organization. Fields a kind does not use are cleared on save.

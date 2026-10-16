# A visitor's data request (audit 2026-10-15, GDPR-1)

The organization controls its visitors' data and FLOX processes it. When
a visitor asks what is held about them, or asks for it to be deleted,
the organization answers from Settings → Visitor data.
`apps/internal/visitordata`.

## Find

`GET /privacy/visitor?clickId=…` or `?ip=…` (`settings.write`: the answer
shows IP addresses). Scoped to the organization.

- **By IP:** every click from the address, up to 500 (`truncated` beyond
  that: an office or carrier NAT is many people). IPs are kept for
  `RETENTION_IP_DAYS`, so an older click is found only by its click id.
- **The report** for those click ids:
  - clicks: time, campaign, geo, device, IP, user agent, referrer;
  - visits and other events;
  - conversions;
  - push subscriptions (the endpoint's host only — the URL is a sending
    credential);
  - Telegram users;
  - the number of pixel sends.

  The panel offers it as JSON to hand to the visitor.

## Erase

`POST /privacy/visitor/erase {clickIds}` (`settings.write`, 1–500 ids;
the audit trail records the request).

- **Deleted:**
  - ClickHouse `click_events`, `tracking_events`, `click_lookup` (waited
    for, first — a failure leaves Postgres untouched and the request can
    be repeated);
  - Postgres `push_subscriptions`, `telegram_users`, `telegram_joins`,
    `telegram_invite_links`, `pixel_deliveries` (their payload carries IP
    and user agent).
- **Kept:** conversions (`conversion_events`, `ltv_events`, the
  `postbacks` ledger), outgoing postbacks and the postback log. They are
  the organization's business records — invoices and disputes rest on
  them — and name the visitor only by a pseudonymous click id. Daily
  aggregates keep their counts (no personal data).
- Reports lose the erased clicks. A conversion that arrives later for an
  erased click is recorded as `unknown_click`.

## Referrer

`referrer` expires with IP and user agent (`RETENTION_IP_DAYS`, column
TTL, `chstore.SetPersonalDataTTL`): it can carry the referring page's
query string. Only the click log shows it.

## Tests

`visitordata_test.go` (Postgres + ClickHouse):
- by IP finds this organization's clicks from that IP, not another IP's
  or another organization's;
- by click id the report has clicks, events, conversions, push
  subscription;
- erasure deletes clicks, visits and lookup rows and the push
  subscription, keeps the conversion and the other visitor's and
  organization's clicks.

Mutation-checked: without the organization filter, the IP search fails
the test. `visitor-data-panel.test.tsx`.

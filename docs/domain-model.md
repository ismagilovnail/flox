# Domain Model

Entities as defined by the master spec §7, §17, §35. Full column-level detail
lands with the PostgreSQL migrations (Phase 17); this document is the
conceptual map.

## Identity & tenancy

- **organization** — the tenant. Every tenant-scoped table has
  `organization_id NOT NULL`, filtered in every query (repository layer),
  never trusted from the request body. See §36-TENANCY.
- **user**, **membership**, **role**, **permission** — RBAC. Roles: Owner,
  Admin, Manager, Buyer, Analyst, Viewer (Phase 28).

## Routing (the core control-plane object graph)

```
Campaign
    → Stream Set   (priority-ordered, first match wins, filters, pixels)
        → Filter Group   (AND/OR, nested)
            → Filter Condition
        → Flow   (weighted)
            → destination: landing / pwa / postlanding / offer
    → fallback / safe destination (if no set matches)
```

- **campaign** — top-level tracking entity; owns stream sets and a fallback
  destination.
- **stream_set** — ordered ruleset (priority) with filters, flows, pixels.
- **filter_group** / **filter_condition** — AND/OR nested rule tree over
  request attributes (geo, device, bot/proxy, UTM, subs, etc.) — see
  [`routing.md`](routing.md).
- **flow** — weighted destination binding: cpa/network + offer + offer-URL,
  optional pwa, optional landing, optional postlanding.

## Traffic sources & offers

- **traffic_source** — where clicks originate; tracking template, cost
  integration.
- **network** — affiliate/CPA network; postback config.
- **offer** / **offer_link** — Network → Offer → Offer Link hierarchy. Offer
  links use the shared macro resolver (`{click_id}`, `{status}`, `{revenue}`,
  `{currency}`, `sub1..10`, …).

## Destinations

- **landing** (internal/external), **pwa** (manifest, icon, theme, start
  URL), **postlanding**.

## Tracking & attribution

- **domain**, **tracking_link** — resolve `GET /t/:tracking_id` to a
  campaign and generate a `click_id` (ULID).
- **pixel**, **postback** (incoming + outgoing) — conversion signal
  ingestion/egress. Dedup key `(click_id, status)`, not `click_id` alone —
  see [`event-model.md`](event-model.md).

## Cost & currency

- **cost_entry** — manual or imported ad-spend, scoped by
  campaign/source/day/country (§27-COST).
- **fx_rate** — `(currency, date) → rate_to_usd`, used to normalize revenue
  and cost at event time, never at query time (§50-FX).

## Secondary modules (post-core)

- **tag** + **taggable** (polymorphic join: tag_id, entity_type, entity_id,
  organization_id) — cross-entity tagging (§30.6).
- **custom_metric** — team-private formula over the metrics registry
  (§30.5).
- **report_preset** — saved `{columns, metrics, grouping, period, timezone}`
  (§27.5).
- **referral**, **referral_balance**, **referral_transaction** — referral
  program (§30.7).
- **gallery_item** — content gallery, system-provided or team-uploaded
  (§30.8).

## Audit

- **api_key**, **audit_log** — API access and change history.

All primary keys use **ULID** (one standard, consistently, everywhere — not
UUID/ULID mixed). `created_at`, `updated_at`, `organization_id` are present
on every tenant-scoped table.

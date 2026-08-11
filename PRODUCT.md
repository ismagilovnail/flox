# PRODUCT

## What FLOX is

FLOX is a production-grade SaaS platform for traffic tracking, campaign
management, traffic classification, configurable routing, conversion
tracking, attribution, postbacks, cost tracking, analytics, reporting, and
LTV/cohort analysis — aimed at performance/affiliate marketing teams,
including iGaming CPA verticals where deposit lifecycle (HOLD → ACCEPT →
REDEP) and LTV are core to profitability.

Target functional parity with the documented capabilities of established
products in this category (e.g. the concepts exposed by Adset.Pro
documentation), with FLOX's own architecture, branding, UX, and
implementation. No proprietary source code, UI assets, or branding are
copied.

**Tagline:** Track. Route. Optimize.

## Core capability areas

```
traffic tracking            campaign management        traffic classification
configurable routing        stream sets                 filters
flows                       landing pages                PWA
postlanding                 offer management              affiliate networks
conversion tracking         attribution                   postbacks
pixels                      traffic-source integrations   cost tracking
analytics                   reporting                     LTV & cohort analysis
domains                     teams                          RBAC
API
```

Secondary modules (built after the core control plane): tags (cross-entity),
custom metrics builder, report presets + directory stats, referral program,
content gallery.

## Core workflows (priority order)

These are the workflows the product must make obviously easy — they drive
every UX and API decision:

```
1. Create Campaign
2. Create Stream Set
3. Build Filters
4. Create Flow
5. Simulate Routing
6. View Analytics
7. Track Conversion (HOLD → ACCEPT → REDEP)
8. Configure Postback
9. Read LTV / Cohorts
```

Secondary (after the core nine): tag & filter entities, define and use a
custom metric, save/apply a report preset and drill in from a directory,
refer a user and track balance, browse & apply gallery content.

## Design principle

Not "another admin dashboard." FLOX is a professional traffic infrastructure
platform with an exceptional, explainable control plane: routing decisions
must always be able to answer *why did this match / why this flow / why
fallback / was sticky applied and from where*. Information-dense,
trading-terminal aesthetic — not a generic marketing dashboard. See
`CLAUDE.md` "UX FLOOR" and the master spec §68 design quality bar.

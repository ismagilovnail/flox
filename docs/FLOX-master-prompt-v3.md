# FLOX — MASTER BUILD PROMPT FOR CLAUDE CODE (v3)

> Revision note (v2): domain-model gaps fixed vs v1. Changes concentrated in
> event model (§43), routing/sticky semantics (§39, §39-STICKY), LTV/cohorts
> (new §26.5 + metrics registry §50), postback dedup (§45), cost ingestion
> (new §27-COST), multitenancy invariant (new §36-TENANCY), regex safety (§22),
> PWA WebView vs moderator-detection split (§73), currency conversion, and a
> resolved phase-ordering conflict for the routing simulator (§10, §26).
>
> Revision note (v3): documentation-parity gaps closed vs guide_en. Added as
> explicit modules/phases: Custom Metrics as a full formula builder (new §30.5),
> Tags across all entity types (new §30.6), Report Presets + "view stats from
> directory" (new §27.5), Referral Program (new §30.7), Content Gallery (new
> §30.8), and the FB "unfilled subs" tracking nuance (§42 note). Sidebar (§17),
> dev order (§9), product priority (§84), docs (§76) and DoD lists updated to
> match. Everything from v1/v2 preserved.

## 0. ROLE

You are the lead architect, senior product engineer, UX engineer, frontend engineer, backend Go engineer, database engineer, QA engineer, and DevOps engineer responsible for building **FLOX**.

FLOX is a production-grade SaaS platform for:

* traffic tracking;
* campaign management;
* traffic classification;
* configurable routing;
* stream sets;
* filters;
* flows;
* landing pages;
* PWA;
* postlanding;
* offer management;
* affiliate networks;
* conversion tracking;
* attribution;
* postbacks;
* pixels;
* traffic-source integrations;
* cost tracking;
* analytics;
* reporting;
* LTV & cohort analysis;
* domains;
* teams;
* RBAC;
* API.

The product should achieve functional parity with the documented capabilities of products in this category, including the concepts exposed by Adset.Pro documentation, while having its own architecture, branding, UX, and implementation.

Do NOT copy proprietary source code, proprietary UI assets, or branding.

Product name:

# FLOX

Tagline:

# Track. Route. Optimize.

---

# 1. MOST IMPORTANT RULE

## DO NOT BUILD EVERYTHING AT ONCE.

You must work strictly in phases.

Never jump to a later phase because it seems easy.

Never generate a huge amount of code before validating the current phase.

For every phase:

```text
1. Inspect
2. Plan
3. Implement
4. Run
5. Test
6. Fix
7. Review
8. Document
9. Stop
```

Only continue to the next phase when the current phase passes its acceptance criteria.

---

# 2. INTERACTION PROTOCOL

At the beginning of every phase:

```text
PHASE X
NAME
GOAL
```

Then provide:

```text
What will be built
Files that will be created/changed
Dependencies required
Acceptance criteria
```

Then implement it.

After implementation:

```text
VALIDATION
```

Run:

```text
typecheck
lint
unit tests
build
```

where applicable.

Then report:

```text
COMPLETED
TESTS
KNOWN ISSUES
FILES CHANGED
NEXT PHASE
```

Do NOT ask for permission to continue unless a real architectural blocker exists.

However:

**Do not silently make destructive changes.**

---

# 3. EXISTING REPOSITORY RULE

Before writing code:

Inspect the repository.

Check:

```text
package.json
pnpm-lock.yaml
npm lockfiles
tsconfig
next.config
tailwind config
src/
app/
components/
public/
README
Docker
.env.example
```

If a project already exists:

* preserve useful existing code;
* do not rewrite everything;
* identify the existing architecture;
* improve it incrementally.

If the repository is empty:

initialize the project according to the stack defined below.

---

# 4. TECHNOLOGY STACK

## Frontend

Use:

```text
Next.js
React
TypeScript
Tailwind CSS
shadcn/ui
Radix UI
TanStack Query
TanStack Table
React Hook Form
Zod
Zustand
Lucide
date-fns
```

Charts:

```text
Apache ECharts
```

Use a consistent chart abstraction.

---

# 5. BACKEND

Backend must be Go.

Use:

```text
Go
PostgreSQL
ClickHouse
Redis
S3-compatible object storage
```

API:

```text
REST
OpenAPI
JSON
```

Use:

```text
chi
pgx
sqlc where appropriate
goose or equivalent migrations
zerolog or slog
OpenTelemetry
```

Do not introduce unnecessary frameworks.

Prefer standard Go patterns.

> **Regex safety (applies wherever user-supplied patterns are evaluated):**
> Use ONLY Go's standard `regexp` package (RE2, no catastrophic backtracking).
> Never pull in a PCRE-compatible library for user-defined filter patterns.
> Enforce a compiled-pattern length cap and reject patterns that fail to compile
> at save time — never at request time on the hot path.

---

# 6. ARCHITECTURE

Initially build the backend as a:

# MODULAR MONOLITH

Do NOT start with microservices.

Logical modules:

```text
auth
organization
users
campaign
traffic_source
routing
classifier
tracking
attribution
conversion
postback
pixel
landing
pwa
postlanding
offer
network
domain
analytics
reports
ltv
push
cost
audit
integration
```

The tracker and asynchronous workers can later be extracted into independent services.

> **Resolved conflict (v1 §35 vs §6):** From day one, `apps/tracker` and
> `apps/worker` exist as SEPARATE BINARIES but live INSIDE the same Go module
> and SHARE the internal packages (routing, classifier, domain). This is still a
> modular monolith at the code level (one module, shared domain logic) while
> allowing the tracker to be deployed and scaled independently. Do not duplicate
> routing/classification logic between binaries — both import `internal/routing`
> and `internal/classifier`.

---

# 6-SHARED. SHARED DOMAIN LOGIC (CRITICAL — READ BEFORE PHASE 2)

Certain logic MUST have exactly ONE implementation, consumed by both the UI
(routing simulator, filter builder previews) and the backend tracker:

```text
routing decision (stream-set priority, AND/OR filter eval, weighted pick, sticky)
filter evaluation semantics
metric formulas
```

To prevent two divergent implementations, decide the sharing strategy in Phase 0
and record it in ARCHITECTURE.md. Acceptable strategies:

```text
A) Go core is the single source of truth. The routing simulator (Phase 10) is a
   thin UI over a real backend endpoint POST /routing/simulate. During the
   frontend-first phases the simulator runs against a local mock of THAT SAME
   contract, and is switched to the real endpoint in Phase 27. The simulator is
   NOT a second routing implementation in TypeScript.

B) Filter-evaluation and weight-normalization pure functions are authored once
   in a shared spec (documented in docs/routing.md) with a conformance test
   fixture that BOTH the Go engine and any TS preview must pass identically.
```

Prefer (A). Whichever you choose, there must be a single conformance fixture
(a table of inputs → expected route decisions) that both sides are tested
against. This is a core FLOX correctness guarantee, not an optional nicety.

---

# 7. DATA ARCHITECTURE

## PostgreSQL

Use PostgreSQL for:

```text
users
organizations
teams
roles
permissions

campaigns
stream_sets
filters
flows

sources
networks
offers

landings
pwas
postlandings

domains
tracking_links

pixels
postbacks

cost_entries        (manual + imported ad-spend, see §27-COST)
fx_rates            (currency → USD by date, see §50-FX)

integrations
api_keys

audit_logs
```

---

## ClickHouse

Use ClickHouse for high-volume event data:

```text
clicks
tracking_events
conversion_events
cost_events
postback_events
ltv_events          (or a materialized view driving cohort/LTV, see §26.5)
```

and analytical aggregates.

---

## Redis

Use Redis for:

```text
cache
sticky assignments (CACHE ONLY — not source of truth, see §39-STICKY)
rate limits
short-lived sessions
job coordination
postback dedup keys (see §45)
```

---

# 8. CORE PRINCIPLE

FLOX has four major layers:

```text
CONTROL PLANE
      ↓
TRACKING ENGINE
      ↓
EVENT / ATTRIBUTION ENGINE
      ↓
ANALYTICS ENGINE
```

Control plane:

```text
campaigns
flows
filters
offers
sources
domains
costs
configuration
```

Tracking engine:

```text
incoming request
classification
routing
redirect
```

Event engine:

```text
click
event
conversion
postback
attribution
```

Analytics:

```text
metrics
dimensions
reports
dashboards
LTV / cohorts
```

---

# 9. DEVELOPMENT ORDER

This is mandatory.

```text
PHASE 0
Repository inspection

PHASE 1
Product foundation

PHASE 2
Design system

PHASE 3
Application shell

PHASE 4
Dashboard

PHASE 5
Analytics UI

PHASE 6
Campaign UI

PHASE 7
Stream Set UI

PHASE 8
Filter Builder

PHASE 9
Flow Builder

PHASE 10
Routing Simulator

PHASE 11
Offers / Networks / Sources UI

PHASE 12
Landing / PWA / Postlanding UI

PHASE 13
Conversions / Postbacks / Pixels UI

PHASE 14
Domains / Team / Settings

PHASE 14.5
Tags (cross-entity)        ← NEW (v3)

PHASE 14.6
Custom Metrics builder     ← NEW (v3)

PHASE 14.7
Report Presets + directory stats  ← NEW (v3)

PHASE 14.8
Referral Program           ← NEW (v3)

PHASE 14.9
Content Gallery            ← NEW (v3)

PHASE 15
Frontend integration architecture

PHASE 16
Go backend foundation

PHASE 17
PostgreSQL schema

PHASE 18
Campaign API

PHASE 19
Routing engine

PHASE 20
Traffic classifier

PHASE 21
Tracking engine

PHASE 22
Attribution engine

PHASE 23
Conversion engine

PHASE 24
Postback engine

PHASE 25
Analytics pipeline

PHASE 26
ClickHouse analytics

PHASE 26.5
LTV & cohort engine        ← NEW

PHASE 27
Frontend/backend integration

PHASE 27-COST
Cost ingestion             ← NEW (manual first, ad-network import later)

PHASE 28
Authentication / RBAC + tenant isolation

PHASE 29
Observability

PHASE 30
Security hardening

PHASE 31
Performance optimization

PHASE 32
E2E testing

PHASE 33
Production deployment

PHASE 34
Final audit
```

---

# 10. PHASE 0 — REPOSITORY INSPECTION

First task.

Do not implement product features.

Inspect repository.

Produce:

```text
Current architecture
Existing dependencies
Existing UI
Existing configuration
Potential conflicts
Recommended structure
Shared-domain-logic strategy decision (§6-SHARED: choose A or B)
```

If empty:

create:

```text
apps/
packages/
docs/
infra/
```

Recommended frontend:

```text
apps/web
```

Backend:

```text
apps/api
```

Tracker:

```text
apps/tracker
```

Worker:

```text
apps/worker
```

Shared:

```text
packages/ui
packages/config
packages/types
```

Do not create unnecessary packages.

Acceptance:

* repository understood;
* architecture documented;
* shared-domain-logic strategy chosen and written to ARCHITECTURE.md;
* no destructive changes.

---

# 11. PHASE 1 — PRODUCT FOUNDATION

Create product metadata:

```text
FLOX
Track. Route. Optimize.
```

Create:

```text
README
ARCHITECTURE.md
PRODUCT.md
ROADMAP.md
```

Document:

```text
domain model
architecture
development phases
environment variables
local development
event model (§43 — full list)
routing model (stream sets, sticky, fallback)
```

Create `.env.example`.

---

# 12. PHASE 2 — DESIGN SYSTEM

This is the first major visual phase.

Do NOT build business logic.

Create the complete FLOX design system.

Design principles:

```text
modern
premium
technical
fast
dense
minimal
professional
```

Visual references (for QUALITY BAR, not for copying a look):

```text
Linear
Vercel
Stripe
Raycast
modern trading terminals
```

Do not copy them.

> **Identity anchor (avoid the generic AI-SaaS default):** The dark-first +
> Linear/Vercel aesthetic is exactly where generated UIs converge. FLOX's
> differentiator is INFORMATION DENSITY, like a trading terminal or Grafana, not
> a marketing dashboard. Concretely: tables and live numbers are the hero, not
> big cards; default to compact rows scannable 20+ at a time without scrolling;
> tabular/mono numerals for all metrics and IDs; one restrained accent color;
> semantic status colors carry meaning (ROI up / CR down must read in <0.5s).
> If a screen looks like a generic admin template with oversized stat cards,
> it has failed the brief — revise before proceeding.

---

# 13. DESIGN SYSTEM RULES

Default theme:

# DARK FIRST

Also support light mode.

Use:

```text
neutral surfaces
subtle borders
high information density
strong typography hierarchy
small radius
restrained shadows
```

Avoid:

```text
huge gradients
excessive glassmorphism
oversized cards
random colors
unnecessary animation
generic dashboard templates
```

---

# 14. TYPOGRAPHY

Use a modern sans-serif.

Hierarchy:

```text
Display
H1
H2
H3
Body
Small
Caption
Mono
```

Numbers should use tabular numerals where appropriate.

Metrics must be visually easy to scan.

---

# 15. COLOR SYSTEM

Semantic colors:

```text
success
warning
danger
info
neutral
```

Do not hardcode colors inside components.

Use design tokens.

---

# 16. UI COMPONENT LIBRARY

Create reusable:

```text
Button
IconButton
Input
Select
Combobox
Checkbox
Radio
Switch
Textarea

Dialog
Drawer
Popover
Tooltip
Dropdown

Tabs
Badge
Tag
Avatar

Card
StatCard

DataTable
DataGrid

EmptyState
ErrorState
LoadingState

Skeleton

DateRangePicker

CommandMenu

Breadcrumbs

Pagination

ChartCard

FilterChip
FilterGroup
```

Everything must be reusable.

---

# 17. PHASE 3 — APPLICATION SHELL

Build:

```text
Sidebar
Topbar
Breadcrumbs
Command Menu
User Menu
Workspace Selector
Notifications
```

Sidebar:

```text
Overview

Analytics

Campaigns
Traffic Sources
Offers
Networks

Landings
PWA
Postlanding

Domains

Conversions
Postbacks
Pixels

Reports
LTV / Cohorts
Push

Referral
Content Gallery

Team
Settings
```

> Tags are not a sidebar page — they are a cross-entity control surfaced inside
> Campaigns / Offers / Networks / Flows / Sources / PWA / Landings lists (§30.6).
> Custom Metrics and Report Presets live under Settings and inside the report
> builder (§30.5, §27.5).

Sidebar states:

```text
expanded
collapsed
mobile
```

Keyboard:

```text
⌘K
```

---

# 18. PHASE 4 — DASHBOARD

Build realistic mock data.

Do not connect backend yet.

Dashboard:

```text
Revenue
Spend
Profit
ROI
Clicks
Conversions
CVR
CPA
```

> **Note on Spend/Profit/ROI:** these depend on cost data (§27-COST). In the
> mock phase, use plausible mock spend. In production, if no cost is present for
> a slice, the UI must show profit/ROI as "—" (no cost) rather than silently
> treating cost as 0 and reporting a falsely positive ROI.

Charts:

```text
Revenue
Spend
Profit
Conversions
```

Tables:

```text
Top campaigns
Top offers
Top countries
Top flows
```

The dashboard must look production-ready.

---

# 19. PHASE 5 — ANALYTICS

Build the analytics explorer.

UI:

```text
Date range
Timezone
Dimensions
Metrics
Filters
Group By
Sort
Compare
```

Dimensions:

```text
campaign
source
country
region
city
device
platform
os
browser
language
flow
landing
pwa
postlanding
offer
network
```

Metrics:

```text
clicks
unique clicks
conversions
revenue
cost
profit
ROI
ROAS
CTR
CVR
CPC
CPA
EPC
```

Create:

```text
table
line chart
bar chart
funnel
```

Use mock data.

> The funnel must reflect the full event model (§43): SOURCE_CLICK → LAND_VIEW →
> LAND_CLICK → PWA_VIEW → PWA_INSTALL → CPA_HOLD → CPA_ACCEPT → CPA_REDEP, with
> per-step conversion %. Not a generic 3-step funnel.

---

# 20. PHASE 6 — CAMPAIGNS

Create:

```text
Campaign list
Campaign creation
Campaign detail
Campaign overview
Campaign settings
```

List:

```text
Name
Status
Source
Clicks
Conversions
Revenue
Spend
Profit
ROI
Updated
```

Actions:

```text
Open
Pause
Duplicate
Archive
Copy tracking URL
```

---

# 21. PHASE 7 — STREAM SETS

Create visual Stream Set management.

Each Stream Set:

```text
Name
Priority
Status
Filters
Flows
Pixels
Fallback
```

Visual hierarchy:

```text
Campaign
    ↓
Stream Set (ordered by priority, evaluated top-down)
    ↓
Filter group (AND / OR)
    ↓
Flow group (weighted)
```

Support:

```text
drag reorder (priority)
duplicate
enable/disable
```

> Semantics to encode in the UI: stream sets are evaluated top-to-bottom by
> priority; the FIRST matching set wins; if none match, traffic goes to the
> campaign fallback / safe destination. Make "why did this set match / not match"
> legible — this is the same explainability the simulator (Phase 10) surfaces.

---

# 22. PHASE 8 — FILTER BUILDER

This is a critical component.

Build a visual rule engine.

Support:

```text
AND
OR
nested groups
```

Fields:

```text
country
region
city
device
platform
os
os_version
browser
browser_version
language

bot
proxy
asn
connection_type

referrer

utm_source
utm_medium
utm_campaign
utm_content
utm_term

sub1-sub10

external_click_id
```

Operators:

```text
IS
IS_NOT
IN
NOT_IN
CONTAINS
NOT_CONTAINS
STARTS_WITH
ENDS_WITH
MATCHES        (RE2 only — see §5 regex safety; validated at save time)
EXISTS
NOT_EXISTS
GT
GTE
LT
LTE
BETWEEN        (numeric/time ranges, e.g. hour BETWEEN [9,21])
```

UI must feel extremely polished.

> Country codes are ISO-3166 alpha-2 (UK is invalid; the UK is `GB`). Validate
> and surface this in the UI to prevent the classic "filter never matches"
> support ticket. `bot`/`proxy` are boolean-like string flags ("0"/"1") — model
> them as a typed toggle in the UI, not a free-text field.

---

# 23. FILTER BUILDER EXAMPLE

```text
MATCH ALL

  Country
  IS
  [US]

  Device
  IN
  [Mobile, Tablet]

  MATCH ANY

    OS
    IS
    [Android]

    OS
    IS
    [iOS]
```

Add:

```text
+ Condition
+ Group
```

---

# 24. PHASE 9 — FLOW BUILDER

Create visual funnel editor.

Flow:

```text
Landing
   ↓
PWA
   ↓
Postlanding
   ↓
Offer
```

Each node can be:

```text
enabled
disabled
configured
duplicated
```

Flow weight:

```text
50%
30%
20%
```

Show normalized percentages.

> A flow binds: cpa/network + offer + offer-URL, optional pwa (+ pwaType:
> internal/external/ios_app), optional landing (+ landingAsPwa toggle), optional
> postlanding, weight, active flag. Weights are arbitrary integers normalized to
> % by the engine; show both the raw weight and the resulting %.

---

# 25. FLOW BUILDER NODE TYPES

```text
Landing
PWA
Postlanding
Offer
Redirect
Fallback
```

Nodes should support:

```text
configuration
preview
status
analytics summary
```

---

# 26. PHASE 10 — ROUTING SIMULATOR

This is a flagship feature.

> **Architecture (resolves v1 conflict):** The simulator is a THIN UI over the
> routing decision contract, NOT a second routing implementation in TypeScript
> (see §6-SHARED). During frontend-first phases it runs against a local mock that
> implements the exact same request/response contract as the future
> `POST /routing/simulate` endpoint. In Phase 27 it is switched to the real Go
> engine with zero UI changes. The decision logic lives in Go (Phase 19) and is
> covered by the shared conformance fixture.

Inputs:

```text
Country
Region
Device
Platform
OS
Browser
Language
Bot
Proxy
ASN
Connection
Referrer
UTM
Sub parameters
```

Button:

```text
Simulate
```

Result:

```text
Request
↓
Classification
↓
Campaign
↓
Stream Set
↓
Filters
↓
Flow
↓
Destination
```

Show:

```text
matched rule
failed rule
selected flow
fallback
sticky assignment (if it would apply)
```

The simulator must explain the decision.

---

# 27. PHASE 11 — SOURCES / NETWORKS / OFFERS

Build:

```text
Traffic Sources
Networks
Offers
```

> Model the real hierarchy: Network → Offer → Offer Link (URL with macros).
> Macros/placeholders are a cross-cutting substitution system
> (`{click_id}`, `{status}`, `{revenue}`, `{currency}`, `sub1..10`, etc.) used in
> offer links, postback templates, and pixel payloads — implement ONE macro
> resolver, reused everywhere.

Source:

```text
name
type
tracking template
cost integration
status
```

Network:

```text
name
postback
status
```

Offer:

```text
name
network
countries
payout
currency
cap
status
```

---

# 27-COST. PHASE (COST INGESTION) — NEW

Profit / ROI / ROAS are meaningless without spend. Do NOT leave cost as an
afterthought.

MVP (do this first, during control-plane work):

```text
Manual cost entry / CSV import per campaign/source/day/country
cost_entries table in PostgreSQL
cost is joined into analytics as cost_events in ClickHouse (or joined at query)
UI: enter/edit spend by day and dimension
```

Later (integration phase, provider-agnostic via CostProvider interface §74):

```text
Facebook Ads API spend pull (OAuth + dev token flow)
TikTok Ads API spend pull
scheduled sync, currency-normalized to USD via fx_rates (§50-FX)
```

Acceptance: dashboard ROI reflects entered cost; a slice with no cost shows
profit/ROI as "—", never a false-positive ROI computed against zero cost.

---

# 28. PHASE 12 — LANDING / PWA / POSTLANDING

Create:

```text
Landing library
Landing editor
PWA library
PWA editor
Postlanding library
```

Landing:

```text
internal
external
```

PWA:

```text
manifest
icon
name
theme
start URL
```

Postlanding:

```text
name
URL/content
events
```

> **PWA install technical requirement (distinct from any moderation concern —
> see §73):** In-app WebViews (traffic opened inside FB/IG/TikTok/Telegram
> browsers) do NOT fire the install prompt, so the PWA funnel is technically
> broken there. FLOX must be able to bounce such traffic to the external browser
> (Android intent scheme / iOS Safari scheme) so install can occur. This is a
> platform-neutral capability about PWA mechanics — it is NOT vendor-specific
> moderator detection, which §73 forbids. Keep the two separate.

---

# 29. PHASE 13 — CONVERSIONS / POSTBACKS / PIXELS

Build:

```text
Conversions
Incoming Postbacks
Outgoing Postbacks
Pixels
Postback Logs
Event Mapping
```

Conversion detail must have a timeline:

```text
Click
Landing
PWA
Offer
Conversion
Postback
```

---

# 30. PHASE 14 — DOMAINS / TEAM / SETTINGS

Domains:

```text
domain
status
SSL
tracking
```

> Domains is a real module, not a text field: registrar account connection
> (e.g. Namecheap-style provider via interface), DNS/nameserver management
> (Cloudflare-style provider via interface), own-domain parking + verification,
> expiry tracking, automatic SSL issuance for campaign/PWA/fallback domains.
> Store registrar/DNS credentials encrypted; respect provider rate limits.
> Keep providers behind interfaces (§74) — no vendor lock-in.

Team:

```text
members
roles
permissions
activity
```

Settings:

```text
organization
timezone
currency
API keys
integrations
security
custom metrics       (managed here + inline in report builder — §30.5)
```

---

# 27.5. PHASE 14.7 — REPORT PRESETS + DIRECTORY STATS — NEW (v3)

Two report-builder conveniences documented in guide_en, currently thin in the
spec. Build them as first-class, not afterthoughts.

**Report Presets** — a saved, reusable set of {columns, metrics, grouping,
period, timezone}:

```text
- Save the current report builder configuration as a named preset.
- Presets are team-scoped (respect tenant isolation §36-TENANCY).
- Apply a preset to instantly reconfigure the report builder.
- Edit / rename / delete; saved reports referencing a preset keep working.
- A system default preset is available to everyone.
```

**View statistics from directories** — one-click drill-in:

```text
- In CPA Networks, Offers, Flows, and Traffic Sources list views, a
  "View statistics" row action opens the report builder pre-filtered by that
  record, with the current date range, the table's metrics, and grouping by day.
- This is a navigation + pre-filter feature: it hands off a fully-formed report
  query, it does not recompute anything client-side.
```

Acceptance: applying a preset reproduces an identical report; "View statistics"
lands on a correctly pre-filtered report for the chosen entity.

---

# 30.5. PHASE 14.6 — CUSTOM METRICS BUILDER — NEW (v3)

Documented as a full module in guide_en, not a one-liner. Build a real formula
engine over existing metrics.

**Formula builder:**

```text
- Left panel: searchable catalog of available metrics grouped by category;
  clicking inserts the metric token at the cursor.
- Operators: + − × ÷ ( )
- Functions: Division, Empty-if-equal, Condition, Round, Absolute, Minimum,
  Maximum. Show contextual argument hints + an example when the cursor is inside
  a function.
- Live validation: green check = saveable; errors explained in the user's
  language.
- SAFE DIVISION: division by zero yields empty, never an error — the report keeps
  working. This is mandatory, not optional.
- Free-form metric name + group (pick existing group or type a new one).
- Format types (e.g. Number, Percent, Currency).
```

Formula examples to support:

```text
Margin per click : ({revenue} - {cost}) / {clicks}
Bot share        : {bots} / ({bots} + {click_all})   [Percent format]
```

**"Show in" targeting:**

```text
Choose which surfaces expose the metric: report builder, campaigns table,
CPA/offers/flows tables, traffic sources table. A target checkbox is DISABLED
when a formula input isn't available in that surface's data — so a custom metric
never renders "empty zeros" where it cannot be computed.
```

**Lifecycle & governance:**

```text
- Draft / Published — drafts are invisible in reports until published.
- Active toggle — hide from all pickers without deleting; saved reports keep
  working.
- "Where used" — list reports and other metrics depending on this one.
- Deletion blocked while in use → archive instead.
- Stable internal identifier: renaming is safe, formulas/reports reference the id
  not the name.
```

**Role access (ties into RBAC §52):**

```text
Owner, Tech : create/edit/delete ANY team metric.
Lead        : create; manage only own metrics.
Buyer       : use published metrics; cannot create.
```

**Constraints (encode explicitly):**

```text
- One formula draws from a SINGLE data source (push metrics cannot be mixed with
  regular metrics).
- LTV/Cohort metrics are NOT available inside formulas (document as a known limit;
  do not silently allow and produce wrong numbers).
- Metrics are team-private (tenant isolation §36-TENANCY): other teams never see
  them.
```

Acceptance: a published custom metric appears only in chosen surfaces, computes
correctly, is safe against division-by-zero, and is invisible to other orgs.

---

# 30.6. PHASE 14.5 — TAGS (CROSS-ENTITY) — NEW (v3)

A color-label system spanning multiple entity types, documented in guide_en.

**Where tags apply (all of these):**

```text
Campaigns, CPA Networks, Offers, Flows, Traffic Sources, PWA apps, Landing Pages
```

**Capabilities:**

```text
- Create a tag with a name + color; quick-create inline while filtering when a
  typed name doesn't exist yet.
- Assign to a single item (via the Tags column or the row's ⋮ "Manage Tags").
- BULK assign: multi-select rows → "Edit Tags" → apply/confirm. In bulk mode only
  tags present on ALL selected items are pre-shown; newly chosen tags apply to all.
- Filter any list by one or more tags. Multi-tag filter uses OR semantics
  (item shown if it has AT LEAST ONE selected tag).
- Edit tag name/color propagates everywhere it's used; remove from item via
  unchecking.
```

**Display rules:**

```text
- ≤3 tags: shown fully (name + color).
- >3 tags: first 3 + "+N" overflow indicator.
- No tags: an "Add tags" affordance in the column.
- Filter chips show a colored dot per tag.
```

**Data model:** a generic `tags` table + a polymorphic `taggables` join
(tag_id, entity_type, entity_id, organization_id). Tags are team-scoped
(§36-TENANCY). Reuse ONE tag component and one filter across all list views —
do not reimplement per entity.

Acceptance: the same tag UX works identically in all seven sections; bulk apply
and OR-filtering behave as specified; tags never leak across orgs.

---

# 30.7. PHASE 14.8 — REFERRAL PROGRAM — NEW (v3)

Documented as its own section in guide_en.

```text
- Personal referral link / code per user or team.
- Track referred signups and attribute them to the referrer.
- Referral balance with a full earnings history (accruals + adjustments).
- Payout / withdrawal request flow (state machine: pending → approved → paid),
  audit-logged (§54).
- UI: referral dashboard (link, invited count, earnings, history).
```

Keep monetary logic server-side and audited. Referral balances are financial
records — apply the same currency handling (§50-FX) and tenant isolation as the
rest of the platform.

Acceptance: a referred signup credits the correct referrer; balance history is
consistent and immutable except through audited transactions.

---

# 30.8. PHASE 14.9 — CONTENT GALLERY — NEW (v3)

A "Resources" section documented in guide_en: a library of ready-to-use content
(creatives, landing/PWA templates, assets) teams can browse and reuse.

```text
- Browsable gallery with categories / search / preview.
- Items can be pulled into the relevant builder (e.g. a template → landing
  editor, a creative asset → asset library).
- Distinguish system-provided gallery content from team-uploaded content.
- Team uploads respect tenant isolation and object-storage rules (S3).
```

Keep it simple in the first pass: a categorized, searchable, previewable library
with "use this" hand-off into the appropriate builder. No AI generation here
(that is a separate concern).

Acceptance: users can find, preview, and apply a gallery item into the correct
builder; team uploads are private to the org.

---

# 31. PHASE 15 — FRONTEND ARCHITECTURE

Before backend implementation:

Refactor frontend.

Ensure:

```text
components
features
hooks
lib
types
schemas
api
stores
```

are cleanly separated.

Recommended:

```text
src/
  app/
  components/
  features/
  hooks/
  lib/
  stores/
  types/
  schemas/
```

Domain-specific code belongs inside `features`.

---

# 32. FRONTEND API CONTRACT

Create typed API client.

Do not scatter `fetch()` throughout components.

Use:

```text
src/lib/api
```

All server calls must go through API abstractions.

Prepare for OpenAPI-generated types.

---

# 33. PHASE 16 — GO BACKEND FOUNDATION

Create:

```text
apps/api
```

Go module.

Structure:

```text
cmd/
internal/
migrations/
pkg/
```

Create:

```text
HTTP server
configuration
logging
request ID
health endpoint
readiness endpoint
OpenTelemetry
```

Endpoints:

```text
GET /health
GET /ready
```

---

# 34. GO ARCHITECTURE

Use:

```text
handler
service
repository
domain
```

Example:

```text
campaign/
    handler.go
    service.go
    repository.go
    model.go
```

Keep business logic out of handlers.

---

# 35. PHASE 17 — DATABASE

Create PostgreSQL migrations.

Core tables:

```text
organizations
users
memberships
roles
permissions

traffic_sources

campaigns
stream_sets
filter_groups
filter_conditions
flows

landings
pwas
postlandings

networks
offers
offer_links

domains
tracking_links

pixels
postbacks

cost_entries
fx_rates

api_keys
audit_logs
```

Use ULID for all primary keys (choose ULID, not "UUID or ULID" — one standard,
consistently, everywhere). ULID sorts by creation time, which helps index
locality.

Add:

```text
created_at
updated_at
organization_id      (on every tenant-scoped table — see §36-TENANCY)
```

where appropriate.

---

# 36. DATABASE RULES

Use foreign keys.

Use indexes deliberately.

Do not index every column.

Document important indexes.

Use migrations.

Never modify production schema manually.

---

# 36-TENANCY. MULTITENANCY INVARIANT — NEW (CRITICAL)

Data isolation between organizations is a hard security invariant, not a
convention:

```text
Every tenant-scoped table has organization_id NOT NULL.
Every query touching a tenant-scoped table filters by organization_id.
Enforcement lives in the repository layer, not the handler — a handler that
  forgets a WHERE clause must not be able to leak data.
```

Implementation guidance:

```text
- Derive organization_id from the authenticated session / API key, never from
  a client-supplied body/query parameter.
- Prefer a repository pattern where org scope is a mandatory constructor/context
  argument, so "no org scope" is a compile-time or obvious-review failure.
- ClickHouse queries are equally scoped: organization_id is part of the sort key
  and every analytics query filters on it.
- Add a cross-tenant isolation test: org A must receive zero rows from org B for
  every list/analytics endpoint. This test is part of Definition of Done for the
  API phases.
```

---

# 37. PHASE 18 — CAMPAIGN API

Implement:

```text
GET /campaigns
POST /campaigns
GET /campaigns/:id
PATCH /campaigns/:id
DELETE /campaigns/:id
```

Also:

```text
POST /campaigns/:id/duplicate
POST /campaigns/:id/pause
POST /campaigns/:id/activate
```

Validate using domain rules. All queries org-scoped (§36-TENANCY).

---

# 38. PHASE 19 — ROUTING ENGINE

Create:

```text
internal/routing
```

Core interfaces:

```go
type Router interface {
    Resolve(ctx context.Context, req RequestContext) (RouteResult, error)
}
```

RouteResult:

```go
type RouteResult struct {
    CampaignID    string
    StreamSetID   string
    FlowID        string
    Destination   string
    Reason        string
    StickyApplied bool
    ConfigVersion int64
}
```

Routing must be deterministic where configuration requires deterministic behavior.

> This package is the single source of truth for routing decisions and is
> consumed by the tracker, the worker, and the /routing/simulate endpoint.
> It must pass the shared conformance fixture (§6-SHARED).

---

# 39. ROUTING ENGINE ORDER

Conceptually:

```text
request
↓
resolve tracking link
↓
resolve campaign
↓
load configuration (versioned)
↓
in-app WebView check → bounce to external browser if needed (§28 note)
↓
sticky lookup (if enabled) → restore prior flow (and click_id if configured)
↓
classify request
↓
bot / proxy filtering
↓
evaluate Stream Sets top-down by priority
↓
evaluate filters (AND/OR, nested)
↓
select eligible flow
↓
apply weighted selection (pickWeighted)
↓
persist sticky assignment (if enabled)
↓
resolve destination (or fallback / safe destination if no set matched)
↓
return RouteResult
```

Keep routing independent from HTTP handlers.

---

# 39-STICKY. STICKY ASSIGNMENT SEMANTICS — NEW (CRITICAL)

Sticky routing keeps one user on one flow (correct A/B testing). Get the source
of truth right or A/B data silently corrupts:

```text
SOURCE OF TRUTH: a client cookie, e.g. sf_{campaignId} with value
  setId:flowId[:clickId]. It survives Redis eviction, restarts, and cross-session
  returns. Effectively long-lived.

REDIS: cache/acceleration ONLY. Never the authority. If Redis is cold, the
  cookie still yields the correct sticky flow.

CONFIG FLAGS:
  stickyFlow            enable/disable
  stickyFlowKeepClickId reuse the original click_id on return (attribution)
  stickyFlowSkipInactive if true, keep the saved flow even if now inactive;
                         if false, drop the cookie and re-pick
```

If sticky is implemented Redis-only, it "leaks" on eviction and invalidates
experiments without any error surfacing — explicitly forbidden.

---

# 40. PHASE 20 — TRAFFIC CLASSIFIER

Create:

```text
internal/classifier
```

Produce normalized:

```text
country
region
city
device
platform
os
browser
language
bot
proxy
asn
connection
```

Use interfaces for external providers (GeoProvider, ASNProvider, BotDetector).

Do not hard-code third-party vendor logic into routing.

---

# 41. PHASE 21 — TRACKING ENGINE

Create separate tracking application (shares internal packages — §6):

```text
apps/tracker
```

Critical path:

```text
HTTP request
↓
parse
↓
classify
↓
route (internal/routing)
↓
record click/event asynchronously (buffered batch writer → queue)
↓
redirect
```

The redirect path must remain fast.

Do not perform expensive analytics queries synchronously.
Do not block the redirect on event persistence — buffer and flush async.

---

# 42. TRACKING LINK

Example:

```text
GET /t/:tracking_id
```

Generate:

```text
click_id (ULID)
```

Preserve:

```text
utm
sub1-sub10
external click IDs (fbclid, ttclid, external_click_id)
```

Create Click event.

Return routing result.

> **Unfilled FB subs nuance:** a portion of Facebook/Instagram clicks arrive with
> empty sub parameters depending on how the link is opened (in-app WebView vs
> external browser, prefetch, redirect chains). Do NOT assume every click carries
> full subs. Persist whatever arrives, mark missing subs as empty (not "unknown
> campaign"), and expose this in analytics so buyers can see the share of
> subs-less traffic rather than silently miscounting attribution. Surface it as a
> diagnostic (e.g. an "empty subs %" indicator), mirroring the documented
> behavior.

---

# 43. EVENT PIPELINE

Architecture:

```text
Tracker
   ↓
Event Queue
   ↓
Worker
   ↓
ClickHouse
```

## FULL EVENT MODEL (authoritative — do not truncate)

Model these as a typed enum shared across tracker, worker, analytics, and pixels.
The schema (ClickHouse) must accommodate ALL of these from day one; adding event
types later is a migration on live analytics data.

```text
TRAFFIC
  SOURCE_CLICK            click on the tracking link
  SOURCE_FILTER           click blocked by campaign filters (bot/geo/device/…)

LANDING
  LAND_VIEW
  LAND_CLICK
  POSTLANDING_VIEW
  POSTLANDING_CLICK

PWA
  PWA_VIEW
  PWA_OPEN
  PWA_INSTALL
  IOS_INSTALL

PUSH
  NOTIFICATION_REQUEST
  NOTIFICATION_SUBSCRIBE
  NOTIFICATION_DECLINE
  NOTIFICATION_UNSUBSCRIBE
  NOTIFICATION_CLICK

TELEGRAM
  TG_JOIN
  TG_START

CPA CONVERSIONS (status is an enum, NOT a single "conversion" type)
  CPA_HOLD      registration
  CPA_ACCEPT    first deposit / FTD (key conversion)
  CPA_REDEP     re-deposit (drives LTV)
  CPA_DECLINE   rejected
  CPA_TRASH     junk / duplicate
```

Full canonical funnel (PWA + gambling):

```text
SOURCE_CLICK → LAND_VIEW → LAND_CLICK → PWA_VIEW → PWA_INSTALL
            → CPA_HOLD → CPA_ACCEPT → CPA_REDEP
```

All events for a user chain are linked by a single click_id (with
stickyFlowKeepClickId preserving it across returns).

---

# 44. PHASE 22 — ATTRIBUTION

Implement:

```text
click_id
external_click_id
```

Attribution service:

```go
type AttributionService interface {
    AttributeConversion(
        ctx context.Context,
        conversion Conversion,
    ) (Attribution, error)
}
```

Do not invent attribution when there is insufficient evidence.

---

# 45. PHASE 23 — CONVERSION ENGINE

Endpoint:

```text
POST /postback
GET /postback
```

Parse:

```text
click_id
external_click_id
status
revenue
currency
```

Map:

```text
network status → FLOX event (CPA_HOLD / CPA_ACCEPT / CPA_REDEP / CPA_DECLINE / CPA_TRASH)
```

Create conversion event.

## DEDUPLICATION (specify explicitly — money correctness)

```text
DEDUP KEY: (click_id, status)  — NOT click_id alone.
  Rationale: the same click legitimately produces CPA_HOLD, then CPA_ACCEPT,
  then multiple CPA_REDEP. Dedup on click_id alone would drop the deposit after
  the registration. Redeposits are distinguished by an additional event
  identifier (network txn id if provided, else a monotonic sequence), so N
  distinct redeposits are N events, but a re-sent identical one is dropped.

WINDOW: dedup key held in Redis with a LONG TTL (partners re-send deposits with
  hours-to-days delay). Persist a durable unique constraint in ClickHouse/PG as
  the backstop; Redis is the fast path.

acceptDuplicates FLAG: per-network override to intentionally accept duplicates
  when a partner's semantics require it.

CURRENCY: store original currency + revenue AND a USD-normalized value using the
  fx rate at event time (§50-FX). Never normalize with the current rate.
```

Deduplicate. Log every postback (success / error / pending) with replay ability.

---

# 46. PHASE 24 — POSTBACK ENGINE

Outgoing postbacks.

Create:

```text
queue
worker
retry (exponential backoff)
logging
dead-letter state
```

Statuses:

```text
queued
processing
success
failed
retrying
dead
```

Never block the conversion request on outbound partner requests.

---

# 47. PHASE 25 — ANALYTICS PIPELINE

Create:

```text
events
↓
ClickHouse
↓
materialized/aggregate tables
↓
analytics service
↓
REST API
↓
frontend
```

Analytics must never query PostgreSQL for raw high-volume traffic events.

---

# 48. PHASE 26 — CLICKHOUSE

Tables:

```text
click_events
tracking_events
conversion_events
cost_events
postback_events
```

Create appropriate sorting keys and partitioning.

Sort keys must lead with organization_id (tenant isolation §36-TENANCY) and
partition by date. Optimize for:

```text
organization
date
campaign
source
country
flow
offer
```

Heavy aggregations use materialized views (per campaign+day, per GEO+day), not
raw scans on every query.

---

# 26.5. PHASE 26.5 — LTV & COHORT ENGINE — NEW (CORE VALUE FOR iGAMING)

This is a primary reason teams pay for a tracker in this vertical. Do not skip.

Data model:

```text
A dedicated ltv_events table (or materialized view) driving cohort math.
Currency normalized to USD at event time via fx_rates.
```

Two cohort types:

```text
FTD cohorts (by first-deposit date):
  cohort_day / cohort_week / cohort_month
  lifetime_days (days from FTD to last redeposit)

Reg cohorts (by registration/CPA_HOLD date):
  reg_cohort_day / reg_cohort_week / reg_cohort_month
  lifetime_days_from_reg
```

LTV windows (per user, from FTD = CPA_ACCEPT):

```text
ltv_d0      day 0 (initial deposit + same-day redeposits)
ltv_d1_7    days 1–7 (early redeposits)
ltv_d8_30   days 8–30
ltv_d31_90  days 31–90
ltv_total   = d0 + d1_7 + d8_30 + d31_90
ltv_per_ftd = ltv_total / cpa_accept
```

Reg-based equivalents: ltv_reg_d0 … ltv_reg_d31_90, ltv_reg_total, ltv_per_reg.

Additional metrics:

```text
ftd_to_redep_rate = redep_unique / cpa_accept
reg_to_ftd_rate   = cpa_accept / cpa_hold
dep_to_redep      = cpa_redep / cpa_accept
total_deposits    = cpa_accept + cpa_redep
total_deposit_revenue = cpa_accept_revenue + cpa_redep_revenue
```

UI (Reports → LTV / Cohorts):

```text
cohort table with lifetime_days heatmap
filters: campaign, source, offer, cpa/network, country
breakdowns: any of the above + cohort period
EXPLICITLY show "window not yet closed" (e.g. d8_30 incomplete if cohort < 30d)
require ≥ 90-day range for full windows
```

Acceptance: numbers reconcile against fixtures; incomplete windows are visibly
marked, not shown as zero.

---

# 49. ANALYTICS API

Example:

```text
POST /analytics/query
```

Request:

```json
{
  "date_from": "...",
  "date_to": "...",
  "timezone": "UTC",
  "dimensions": ["campaign", "country"],
  "metrics": ["clicks", "revenue", "profit", "roi"],
  "filters": []
}
```

Return:

```text
columns
rows
totals
metadata
```

> Timezone affects day boundaries in aggregation — implement it correctly, not
> as a display-only shift.

---

# 50. METRICS REGISTRY

Never calculate business metrics randomly across the codebase.

Create central registry with documented formulas:

```text
TRAFFIC / PERFORMANCE
  clicks
  unique_clicks
  conversions
  revenue
  cost
  profit          = revenue - cost
  roi             = (revenue - cost) / cost
  roas            = revenue / cost
  ctr
  cvr
  cpc
  cpa
  epc

CPA FUNNEL
  cpa_hold        registrations
  cpa_accept      FTDs
  cpa_redep       redeposits
  cpa_decline
  cpa_trash
  reg_to_ftd_rate
  ftd_to_redep_rate
  dep_to_redep
  total_deposits
  total_deposit_revenue

LTV (see §26.5)
  ltv_d0 / ltv_d1_7 / ltv_d8_30 / ltv_d31_90 / ltv_total / ltv_per_ftd
  ltv_reg_* / ltv_per_reg
  lifetime_days
```

Document formulas. Any metric involving cost must handle "no cost" as null, not
zero.

> Built-in metrics live in this central registry. USER-DEFINED metrics are a
> separate layer built ON TOP of it via the Custom Metrics builder (§30.5) —
> they reference registry metrics by stable id, are team-private, and are subject
> to the single-data-source and no-LTV-in-formula constraints documented there.
> Do not let custom formulas bypass the registry or reach raw columns directly.

---

# 50-FX. CURRENCY NORMALIZATION — NEW

```text
fx_rates table: (currency, date) → rate_to_usd
All revenue/LTV stored in BOTH original currency and USD-normalized value.
Normalization uses the rate at the EVENT date, not the query-time rate, so
historical reports stay stable.
Base reporting currency configurable per organization (default USD).
```

---

# 51. PHASE 27 — FRONTEND INTEGRATION

Replace mock data gradually.

Order:

```text
auth
campaigns
sources
offers
networks
flows
stream sets
filters
tracking
conversions
analytics
ltv / cohorts
postbacks
```

Do not replace the entire frontend at once.

The routing simulator is switched from its mock contract to the real
/routing/simulate endpoint here, with no UI change (§6-SHARED / §26).

---

# 52. PHASE 28 — AUTH / RBAC + TENANT ISOLATION

Implement:

```text
authentication
sessions
organizations
memberships
roles
permissions
tenant isolation (§36-TENANCY) — enforced + tested
```

Roles:

```text
Owner
Admin
Manager
Buyer
Analyst
Viewer
```

Permission examples:

```text
campaign.read
campaign.write
analytics.read
offer.read
offer.write
source.read
source.write
team.read
team.write
settings.write
```

Enforce authorization server-side.

Frontend permissions are only UX.

Cross-tenant isolation test is part of Definition of Done here.

---

# 53. PHASE 29 — OBSERVABILITY

Implement:

```text
structured logging
request IDs
trace IDs
OpenTelemetry
Prometheus metrics
```

Track:

```text
tracking_requests
tracking_latency
routing_latency
event_processing_latency
event_queue_depth
event_loss (enqueued vs persisted)
postback_success
postback_failure
analytics_latency
```

---

# 54. PHASE 30 — SECURITY

Implement:

```text
input validation
rate limiting
secure cookies
CSRF protection where applicable
CORS policy
secret management
API key hashing
API key rotation
RBAC
tenant isolation
audit logging
SQL injection prevention
SSRF protection
URL validation
webhook validation
regex safety (RE2, save-time validation — §5)
encrypted storage of registrar/DNS/ad-network credentials
```

Never log:

```text
passwords
API secrets
tokens
private keys
sensitive credentials
PII in postbacks (hash where used for pixel matching)
```

---

# 55. URL / REDIRECT SECURITY

All externally supplied URLs must be validated.

Prevent:

```text
SSRF
malformed URLs
unsafe schemes
open redirects where not intentionally supported
```

Use explicit allowlists where necessary.

Note: the routing engine performs intentional redirects to configured
destinations — those are allowlisted by configuration, distinct from arbitrary
user-supplied redirect targets, which are validated/blocked.

---

# 56. PHASE 31 — PERFORMANCE

Benchmark:

```text
tracking endpoint
routing
classifier
postback
analytics
```

Target:

```text
tracking p50 < 20ms
tracking p95 < 50ms
```

excluding third-party network latency.

Load test: sustained clicks with zero event loss (enqueued == persisted).

Optimize only after measurement. Do not prematurely optimize.

---

# 57. PHASE 32 — E2E TESTING

Create complete scenario:

```text
Create organization
↓
Create source
↓
Create network
↓
Create offer
↓
Create landing
↓
Create flow
↓
Create Stream Set
↓
Create filter
↓
Create campaign
↓
Enter cost
↓
Generate tracking URL
↓
Click
↓
Route
↓
Record event
↓
Receive conversion (HOLD → ACCEPT → REDEP)
↓
Attribute conversion
↓
Send postback
↓
Analytics + LTV
```

Verify every step.

---

# 58. ROUTING TEST CASES

Must test:

```text
AND
OR
nested groups
priority (first-match wins)
fallback
weighted routing (distribution within 2% of configured weights over 10k picks)
sticky routing (cookie survives Redis flush)
sticky keepClickId
sticky skipInactive true/false
inactive campaigns
inactive flows
inactive offers
missing destinations
invalid tracking links
ISO code mismatch (UK vs GB)
in-app WebView bounce
```

All must pass the shared conformance fixture (§6-SHARED).

---

# 59. CONVERSION TEST CASES

Test:

```text
valid conversion
duplicate conversion (same click_id + status → dropped)
legitimate sequence (HOLD, then ACCEPT, then multiple REDEP → all recorded)
delayed re-send after hours (still deduped)
unknown click
unknown campaign
invalid status
missing revenue
different currency (USD normalization at event time)
acceptDuplicates override
retry
postback failure
postback success
```

---

# 60. ANALYTICS TEST CASES

Verify against known fixtures:

```text
click count
unique click count
conversion count
revenue
cost
profit
ROI (and "—" when cost is absent)
ROAS
CTR
CVR
CPA
EPC
reg_to_ftd_rate
ftd_to_redep_rate
ltv_d0 / ltv_total / ltv_per_ftd
cohort window completeness flags
```

---

# 61. PHASE 33 — PRODUCTION

Create:

```text
Dockerfiles
docker-compose.dev.yml
docker-compose.test.yml
production deployment docs
environment documentation
backup strategy
migration strategy
monitoring
alerts
```

Services:

```text
web
api
tracker
worker
postgres
clickhouse
redis
object-storage
```

---

# 62. PHASE 34 — FINAL AUDIT

Before declaring the project complete, audit:

```text
UX
UI
accessibility
security
tenant isolation
performance
database
API
routing
attribution
analytics
LTV / cohorts
testing
observability
documentation
```

Search for:

```text
TODO
FIXME
mock
dummy
temporary
hardcoded
console.log
debugger
```

Remove or document all production leftovers.

---

# 63. IMPORTANT UX RULES

Every screen must have:

```text
loading state
empty state
error state
success feedback
```

Never leave users with blank content.

Destructive actions require confirmation.

Long-running actions show progress.

Forms must have:

```text
validation
inline errors
disabled submit state
success feedback
```

---

# 64. TABLE RULES

Every important table should support:

```text
sorting
pagination
column visibility
column resizing
search
filters
export where appropriate
```

For large datasets use virtualization.

---

# 65. RESPONSIVE RULES

Desktop is primary.

Must still support:

```text
tablet
mobile
```

Do not simply shrink desktop.

For mobile:

```text
sidebar → drawer
tables → responsive cards or horizontal scroll
multi-column forms → stacked
```

---

# 66. ACCESSIBILITY

Target:

# WCAG 2.2 AA

Ensure:

```text
keyboard navigation
visible focus
ARIA where needed
semantic HTML
screen reader support
reduced motion
sufficient contrast
```

---

# 67. ANIMATION

Use animation sparingly.

Preferred:

```text
150–250ms
ease-out
```

Animate:

```text
drawer
modal
popover
page transitions
table updates
status changes
```

Do NOT animate everything.

---

# 68. DESIGN QUALITY BAR

Before completing a frontend phase ask:

```text
Does this look like a $100M SaaS product?
Does it read as a dense, technical control plane rather than a generic admin?
Does spacing feel intentional?
Are tables readable and scannable 20+ rows at a glance?
Are metrics immediately scannable (tabular numerals, status colors meaningful)?
Are interactions obvious?
Are empty states useful?
Are errors understandable?
Does the UI feel fast?
```

If not, improve it before proceeding.

---

# 69. CODE QUALITY

Prefer:

```text
small modules
strong types
explicit interfaces
predictable naming
pure functions
dependency injection
testable business logic
```

Avoid:

```text
god components
god services
global mutable state
duplicated business logic
magic numbers
magic strings
```

---

# 70. FRONTEND RULE

Do not put business logic into React components if it belongs to a domain service.

Bad:

```text
component calculates routing rules
```

Good:

```text
routing decision comes from the shared engine/contract (§6-SHARED);
UI consumes the result and explains it
```

---

# 71. BACKEND RULE

Handlers should be thin.

Bad:

```text
HTTP handler → SQL → routing → business logic → response
```

Good:

```text
handler → service → repository
```

Repositories are org-scoped by construction (§36-TENANCY).

---

# 72. ROUTING RULE

Routing must be deterministic and explainable.

Every route decision should be capable of answering:

```text
Why did this traffic match?
Why did another Stream Set not match?
Why was this Flow selected?
Why was fallback used?
Was a sticky assignment applied, and from where?
```

This is a core FLOX differentiator.

---

# 73. SAFE DESTINATION / FALLBACK MODEL

The platform supports configurable fallback/safe destinations as part of the
routing model. Treat them as generic destination types.

> **Two distinct concepts — keep them separate:**
>
> 1. FORBIDDEN: vendor-specific moderator detection (e.g. "detect this ad
>    network's reviewer and cloak"). Do not embed any specific advertising
>    platform's moderator/QA detection. Keep classification provider-agnostic.
>
> 2. ALLOWED and REQUIRED: generic, provider-neutral routing capabilities such
>    as filter-based fallback destinations, and bouncing in-app WebView traffic
>    to the external browser so the PWA install prompt can fire (§28). This is a
>    technical necessity for the PWA funnel, not moderator cloaking.
>
> Do not conflate the two. Implement (2); do not implement (1).

Do not hard-code behavior for any specific advertising platform.

---

# 74. EXTENSIBILITY

All external integrations should use interfaces.

Examples:

```go
type GeoProvider interface {}
type ASNProvider interface {}
type BotDetector interface {}
type CostProvider interface {}      // manual + FB/TikTok ad-spend
type ConversionProvider interface {}
type PixelProvider interface {}
type DomainRegistrarProvider interface {}   // Namecheap-style
type DnsProvider interface {}                // Cloudflare-style
type FxRateProvider interface {}
```

This allows vendors to be replaced.

---

# 75. NO VENDOR LOCK-IN

Do not make core FLOX logic dependent on:

```text
one geo vendor
one analytics vendor
one affiliate network
one traffic source
one registrar / DNS provider
one cloud provider
```

Adapters belong at integration boundaries.

---

# 76. DOCUMENTATION

Maintain:

```text
docs/
  architecture.md
  domain-model.md
  event-model.md          ← full §43 event list + funnel
  routing.md              ← stream sets, sticky (§39-STICKY), fallback, conformance fixture
  attribution.md
  analytics.md
  ltv.md                  ← cohorts, windows, formulas
  metrics.md              ← metric registry + formulas
  cost.md                 ← manual + ad-network ingestion
  domains.md              ← registrar/DNS providers
  multitenancy.md         ← isolation invariant + tests
  custom-metrics.md       ← formula builder, functions, constraints, roles
  tags.md                 ← cross-entity tagging model
  referral.md             ← referral accrual + payout flow
  api.md
  local-development.md
  deployment.md
  security.md
```

Update documentation after architectural changes.

---

# 77. CHANGELOG

Maintain:

```text
CHANGELOG.md
```

After each meaningful phase:

```text
Added
Changed
Fixed
```

---

# 78. GIT COMMITS

Use logical commits.

Example:

```text
feat(ui): create FLOX design system
feat(campaigns): add campaign management UI
feat(routing): add filter builder
feat(routing): add sticky assignment engine
feat(ltv): add cohort engine
feat(api): add campaign endpoints
feat(tracker): add tracking endpoint
```

Do not create one giant commit for the entire application.

---

# 79. DEFINITION OF DONE

A phase is DONE only when:

```text
✓ implementation complete
✓ no TypeScript errors
✓ no Go compilation errors
✓ lint passes
✓ tests pass
✓ build passes
✓ UX reviewed
✓ responsive behavior checked
✓ loading state exists
✓ empty state exists
✓ error state exists
✓ tenant isolation verified (for API/data phases)
✓ documentation updated
```

---

# 80. WHAT YOU MUST NOT DO

Never:

```text
build all phases simultaneously
replace the whole repository without inspection
invent dependencies unnecessarily
duplicate business logic (esp. a second routing impl in TS)
hardcode production secrets
commit .env
skip tests
skip validation
ignore TypeScript errors
ignore Go errors
create fake APIs that look production-ready
hide errors
dedup conversions on click_id alone
treat missing cost as zero
store sticky assignment only in Redis
truncate the event model
leak data across organizations
embed vendor-specific moderator detection
let a custom-metric formula divide-by-zero into an error (must be empty)
mix push and regular metrics in one custom formula
reimplement tags per entity instead of one shared component
treat empty FB subs as "unknown campaign"
```

Do not create fake functionality and call it complete.

Mocks are allowed only during frontend-first phases and must be explicitly
replaceable, and must implement the same contract as the real backend.

---

# 81. CURRENT EXECUTION RULE

When this prompt is first provided:

DO ONLY:

# PHASE 0

Inspect the repository.

Do not implement the application yet.

After inspection:

1. explain the current repository;
2. propose the exact project structure;
3. identify conflicts;
4. identify what can be reused;
5. choose the shared-domain-logic strategy (§6-SHARED);
6. provide the implementation plan for Phase 1.

Then STOP.

---

# 82. AFTER PHASE 0

Proceed phase-by-phase.

For each phase:

```text
PHASE → PLAN → IMPLEMENT → RUN → TEST → FIX → REVIEW → DOCUMENT → STOP
```

Never skip the validation stage.

---

# 83. IMPORTANT

The user wants:

```text
frontend first
design first
excellent UX
then personal account
then backend
then tracking engine
then analytics
then LTV / cohorts
```

Respect this order.

Do not start Go backend before the frontend architecture and core product
workflows have stabilized.

> Caveat: the routing/filter/sticky/metric DECISION LOGIC is defined in Go and
> the shared conformance fixture (§6-SHARED). Frontend-first means the UI and
> UX come first and run on mock contracts — it does NOT mean re-implementing
> routing in TypeScript. The simulator consumes the contract, not a parallel
> engine.

---

# 84. PRODUCT PRIORITY

Prioritize these workflows above everything:

```text
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

These are the core FLOX workflows.

> Secondary but documented workflows (v3), build after the core nine:
> tag & filter entities (§30.6), define a custom metric and use it in a report
> (§30.5), save/apply a report preset and drill in from a directory (§27.5),
> refer a user and track the balance (§30.7), browse & apply gallery content
> (§30.8). These are convenience/organizational layers over the core control
> plane, not prerequisites for it.

---

# 85. FINAL PRINCIPLE

Do not build FLOX as:

```text
"another admin dashboard"
```

Build it as:

```text
A professional traffic infrastructure platform
with an exceptional control plane.
```

The UI must make complex traffic configuration understandable.

The backend must make routing predictable and explainable.

The event system must make attribution reliable (full event model, correct dedup).

The analytics system must make performance obvious (including LTV over 90 days).

The entire product must feel:

```text
fast
precise
technical
premium
trustworthy
```

# START NOW

Execute:

# PHASE 0 — REPOSITORY INSPECTION

Do not implement later phases yet.

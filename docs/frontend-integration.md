# Frontend/Backend Integration (Phase 27, §51)

§51's literal phase order ("auth, campaigns, sources, offers, networks,
flows, stream sets, filters, tracking, conversions, analytics, ltv/cohorts,
postbacks") assumes backend APIs that were never built — `ROADMAP.md` has
exactly one dedicated backend-API phase before this one (18, "Campaign API"),
and auth doesn't exist until Phase 28. Rather than silently absorbing eleven
phases' worth of missing backend work into "Phase 27," the scope was
negotiated explicitly with the user (two `AskUserQuestion` rounds) down to a
single concrete, honestly-deliverable slice:

> Wire Campaigns CRUD and campaign-detail analytics to the real API. Drop
> anything with no real backend rather than fake it. Everything else stays on
> mocks.

## What's real now

- **`apps/web/src/lib/api/client.ts`** — `apiFetch<T>()`: adds
  `X-Organization-Id` from `NEXT_PUBLIC_DEV_ORG_ID` (the frontend's
  temporary stand-in for auth, mirroring `internal/tenant`'s header — both
  are replaced wholesale by Phase 28), throws `MissingDevOrgError` if unset
  so the failure mode is a clear message, not a silent 400 from the API.
- **`lib/api/{campaigns,traffic-sources,analytics}.ts`** + matching
  `hooks/use-*.ts` (TanStack Query) — campaigns list/get/create/update/
  archive, traffic-sources list, campaign daily click/revenue analytics.
  `Campaign`'s TS type matches the Go JSON exactly: `id, organizationId,
  trafficSourceId, name, status, fallbackUrl, notes, createdAt, updatedAt`.
  No `trackingDomain`, `trackingId`, `clicks`, `conversions`, `revenue`,
  `spend` — those existed only in the old mock model and have no backend
  field, so the columns/inputs that showed them are gone, not faked with
  `"—"` or a client-side placeholder.
- **`apps/internal/trafficsource`** (new, minimal) — `GET
  /traffic-sources`, tenant-scoped, backed by a `traffic_sources` table that
  already existed from an earlier migration but had no read endpoint. The
  one necessary backend addition this phase's scope required — everything
  else it needed already existed (`apps/internal/campaign`'s CRUD handlers,
  `apps/internal/analytics`'s `GET /analytics/campaigns/{id}/daily`).
- **CORS** (`go-chi/cors`, `httpserver.New`) — the Next.js dev server calls
  the Go API cross-origin; `AllowedOrigins` is the API's own `AppURL` config
  value (`APP_URL` env var, defaults to `http://localhost:3000`), not a
  wildcard.
- **`apps/web/.env.example`/`.env.local`** — `NEXT_PUBLIC_API_URL`,
  `NEXT_PUBLIC_DEV_ORG_ID`. Next.js loads env files from `apps/web/`
  itself, not the repo root, so this is a second, separate env surface from
  the root `.env.example` (which covers the Go services). The frontend
  `.gitignore`'s blanket `.env*` pattern was silently swallowing
  `.env.example` too (no `!.env.example` exception, unlike the root
  `.gitignore`) — fixed alongside.
- **`CampaignDetailView`'s Overview tab** — real `StatCard`s (Revenue,
  Clicks, Conversions, CVR) computed from the same two analytics endpoints
  Phase 25/26 built, plus a real daily revenue line chart. `Spend`,
  `Profit`, `ROI`, `CPA` were gone entirely at the time this phase closed
  (not shown as `"—"`: CLAUDE.md invariant #6 reads `"—"` as "a value that
  hasn't been entered yet," and that wasn't true — there was no cost
  pipeline wired to the frontend at all). **Phase 27-COST closed this
  gap** — see `docs/cost-ingestion.md`; all four are real now, with
  Profit/ROI/CPA correctly falling back to `"—"` only when cost genuinely
  isn't entered (or, for CPA, when conversions are zero — a division by
  zero, not a $0.00 acquisition cost).

## Entity lists: limit, offset, total (tech audit 2026-10-06, §5)

`GET /offers`, `/networks`, `/landings`, `/postlandings`, `/pwas`, `/tags`,
`/campaigns`, `/traffic-sources`, `/domains`, `/pixels`, `/event-mappings`
take `?limit=` (1–500, default 500; more is clamped to 500) and `?offset=`
(from 0), and return the page with the list's `total`:
`{"offers": [...], "total": 1203}`. A bad value is a 422. Rows are ordered by
the list's own order with the id as tie-break, so pages neither overlap nor
skip. One contract: Go `internal/pagination`.

apps/web keeps its tables client-side (sort, search, filter, export over the
whole list) and its pickers offer every entity, so every `list*` function in
`src/lib/api` reads all pages through `fetchAllPages`
(`src/lib/api/pagination.ts`): pages of 500 until `total`, a row that moved
between pages kept once. That bounds one response, not the whole list — fine
up to a few thousand entities per organization; beyond that the tables would
need server-side sorting and search.

Not paginated, each bounded by its parent: a campaign's stream sets (the
routing editor needs all of them, in priority order), a campaign's tracking
links, a campaign's cost entries for a date range. Already paginated their
own way: click logs, conversions, postback logs. Small and per user: API
keys, custom metrics, report presets.

Found on the way: `/campaigns` defaulted to 50 rows and the app never asked
for more — every campaign past the 50th was missing from the campaign list
and from every campaign filter (conversions, click logs, LTV, alerts).

## What's still mocked, unchanged

- The standalone `/analytics` report builder — a full ad-hoc report UI
  (dimension/metric pickers, saved views) with no matching backend contract
  at any granularity close to what it renders; narrowing it to fit
  `GET /analytics/campaigns/{id}/daily` would mean rebuilding the UI, not
  integrating it.
- `/ltv-cohorts` — literally a `<PageStub>`; no UI was ever built against
  the Phase 26.5 LTV endpoints to integrate in the first place.
- Postback Logs' **Replay** action — the read-only log itself is real
  now (see below), but replaying an attempt (re-invoking
  `apps/internal/conversion` for an incoming row, or re-enqueuing a
  `apps/internal/postback` delivery for an outgoing one) was deliberately
  scoped out of that phase as a real, separate write capability. See
  `docs/postback-logs.md`.

  **Traffic Sources, Networks, Offers, Stream Sets/Filters/Flows, the
  Routing Simulator, Conversions (list + detail/timeline), Event
  Mappings CRUD, and Postback Logs (read-only) are no longer on this
  list** — full CRUD (or, for the Simulator/Conversions/Postback Logs, a
  real read/compute endpoint) landed for all of them; see
  `docs/traffic-sources.md`, `docs/networks-offers.md`,
  `docs/stream-sets.md`, `docs/routing-simulate.md`,
  `docs/conversions.md`, `docs/event-mappings.md`, and
  `docs/postback-logs.md`. The "Conversions/Postbacks" domain identified
  at the start of this three-phase arc is now fully wired except for the
  deliberately-deferred Replay action.

## Verified

Backend: `go build ./...`, `go vet ./...`, `gofmt -l .` clean; full
`go test ./...` green including the new `trafficsource` package (2 tests).
`curl` against the real running `api` binary confirmed the traffic-sources
list, campaigns list, and a CORS preflight (`OPTIONS` with
`Access-Control-Allow-Origin` echoing `APP_URL`) all work with real seeded
data.

Frontend: `tsc --noEmit` and `eslint` both clean. Full manual browser pass
against the real running `api` + `web` dev servers: `/campaigns` empty
state renders correctly (no more ~50 fake mock rows); New Campaign form's
Source dropdown is populated from the real `GET /traffic-sources` response
("Facebook Ads", "TikTok Ads" — two rows seeded directly into Postgres for
this test); submitting creates a real campaign via `POST /campaigns` and
navigates to its detail page; the Overview tab renders a correct zero-value
empty state (not an error) for a campaign with no conversion events yet;
Settings tab pre-fills from the real record and its Archive control is
present; the campaign list correctly resolves and displays the source name
after creation. The test campaign and its two seed traffic sources were
removed via the real `DELETE /campaigns/{id}` endpoint after verification.

## Deferred to later phases, not silently absorbed

Sources/offers/networks/flows/stream-sets/filters/routing-simulate/
conversions/postbacks all need dedicated backend work before their existing
frontend mocks can be wired up the same way campaigns were — each is its
own phase-sized slice, not a follow-on to this one. §51's phase order can't
be followed literally as written (see the opening paragraph); the practical
order going forward is whatever slice the user picks next, confirmed before
starting, same as this phase was.

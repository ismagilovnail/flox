# Click Log

A read-only, row-level view over `click_events` (§48,
`apps/internal/chstore/schema/001_click_events.sql`) — one row per
`SOURCE_CLICK`/`SOURCE_FILTER` visit, org-wide, newest first. Sits
alongside `apps/internal/analytics`'s aggregate-only reporting: that
package answers "how much traffic, grouped by X"; this one answers "show
me the actual rows," including IP, geo, device/browser/OS, UTM, subs, and
the routing decision (stream set / flow / destination / sticky).

## Why this exists

A user asked whether FLOX captures per-click IP/geo data and, once shown
it does, asked to see it in the UI. There was no such screen — the report
builder and `/overview` only ever expose aggregates
(`docs/analytics-report-api.md`), and no other page does row-level reads
of `click_events` at all.

## Design

Mirrors `apps/internal/postbacklogs` exactly (read side): `Repository`
interface satisfied structurally by `chstore.EventStore`
(`ListClickLog`/`CountClickLog`, `apps/internal/chstore/click_log.go`),
a thin `Service` (`apps/internal/clicklogs/service.go`) with the same
90-day range cap and 50/200 default/max page size as
`postbacklogs.Service`, and a `Handler` mounting a single `GET /` under
`/click-log` (`apps/api/main.go`). No writes, no replay — a click row
isn't a retriable action the way a postback attempt is, so this package
is deliberately narrower than `postbacklogs`.

Frontend: `features/clicklogs/click-log-view.tsx`, a single `DataTable`
page (`app/(app)/click-log`), nav entry under "Traffic" right after
Campaigns. Every column on the schema is reachable via the Columns menu;
only a practical default subset (time, type, campaign, click id, country,
city, device, OS, browser, IP) is visible on first load —
`DataTable` gained an `initialColumnVisibility` prop for this, generic to
any future table with more columns than fit a default view. Campaign IDs
are resolved to names client-side via the existing org-wide
`useCampaigns()` list; stream-set/flow ids are left as raw ids (no
org-wide stream-set listing hook exists — resolving those would mean an
N+1 fetch per campaign, out of scope for a read-only log).

## What this does NOT change

- **Geo/ASN are now populated.** A real provider
  (`apps/internal/classifier/maxmind`, `docs/geoip.md`) is live in the dev
  stack against DB-IP Lite databases in `infra/geoip/` — country/city/ASN
  render real values for real traffic. A deployment without those `.mmdb`
  files in place still falls back to `NoopGeoProvider`/`NoopASNProvider`
  (`—`), same as before.
- **No retention policy.** `click_events` still has no TTL
  (`001_click_events.sql`'s own doc comment) and IP/UA are stored raw,
  unhashed, unanonymized — this page surfaces that data, it doesn't change
  how long it's kept or whether it's identifiable. A retention/anonymization
  policy is a real compliance decision (GDPR-relevant) for the user to make
  explicitly, not something to invent silently here.

## Verified

`go build/vet/test -count=1 ./...` clean, including a real-ClickHouse
integration test (`apps/internal/chstore/click_log_test.go`) covering
list/count, newest-first ordering, pagination, the IP/geo/device/subs
round-trip, and cross-org isolation (CLAUDE.md #5) — a second org sees
zero rows for the first org's clicks. `tsc --noEmit`/`eslint` clean on
`apps/web`. Real browser pass against the actual reporting user's own
account: `/click-log` renders real rows (real internal IPs, resolved
campaign names, device/OS/browser), the Columns menu correctly
toggles every hidden field, no console errors.

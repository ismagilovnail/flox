# Domains + Tracking Links — the missing "where do I send traffic" CRUD

User report: created a campaign, found no way to activate it and no link to
send traffic to. Two separate root causes, both fixed here (activation is
the smaller fix — see this entry's own "Fixed" section below):

1. **Activation**: `campaign-row-actions.tsx`'s list-row menu only offered
   Pause/Resume for a campaign already `active`/`paused` — a brand-new
   `draft` campaign had no quick action at all, only the Settings tab's
   Status dropdown (the backend's `campaign.Service.Activate` already
   handled `draft` fine; this was a frontend gap).
2. **No tracking link, anywhere**: `domains`/`tracking_links` (migration
   00007) have existed since early on, and `apps/tracker`'s hot path
   (`apps/internal/routingstore.ResolveTrackingLink`) already reads them
   for real on every click — but no Go package ever exposed CRUD over
   them, and the frontend's "Domains" page (`features/domains/*`) was pure
   mock data (`lib/mock/domains.ts`/`stores/domains.ts`), with no page at
   all for creating a tracking link. There was genuinely no way, through
   the product, to get a URL to send traffic to.

## What's new

- **`apps/internal/domain`** (new package): `Domain` CRUD —
  `GET/POST /domains`, `GET/PATCH/DELETE /domains/{id}`. Registrar
  connection, DNS record management, and automatic SSL issuance (§30's
  full "real module" vision) are **not** implemented — no registrar/DNS
  adapter exists anywhere in this repo. `status`/`ssl`/`registrar`/
  `dnsProvider` are plain operator-set fields the same honest way
  `campaign.Status` already is, not a "Verify"/"Issue SSL" button
  pretending to check anything real (the mock version had exactly that,
  labeled in its own code as mock — dropped, not carried forward as a fake
  automation). `expiresAt`/`verifiedAt` stay server-computed (always
  `nil`) since nothing here verifies or expires anything yet.
- **`apps/internal/trackinglink`** (new package): the (domain, slug) →
  campaign mapping CRUD — `GET/POST /campaigns/{campaignId}/tracking-links`,
  `PATCH/DELETE .../{id}`. Returns a computed `url` field
  (`http://{domain}/t/{slug}`, matching `apps/tracker/handler.go`'s own
  `GET /t/{trackingID}` route) so the frontend never reconstructs that
  shape itself. Two tenant-isolation checks worth naming since they're the
  actual risk in this package (unlike a plain `List`/`Get`, which are safe
  by construction): `domainId` arrives in the request body and could name
  any domain, not just the caller's own; `campaignId` arrives in the route
  and nothing upstream re-validates its `organization_id` before a
  `Create` (a `List`/`Get`'s `WHERE organization_id = $1` would otherwise
  quietly return zero rows for a wrong-org id — a `Create` can't stay
  quiet about it, since it would otherwise insert a row instead of just
  returning nothing). Both are checked in one INSERT..SELECT rather than
  a separate lookup then insert, so neither check can race a concurrent
  delete/reassignment between the two.
- **`apps/web`**: `features/domains/*` switched from the zustand mock
  store to real API calls (`lib/api/domains.ts`, `hooks/use-domains.ts`) —
  `lib/mock/domains.ts` and `stores/domains.ts` deleted outright, not kept
  alongside. New `features/campaigns/tracking-links.tsx`, mounted at the
  top of a campaign's Overview tab (ahead of the KPI stats — it's the
  first thing a user coming from "create a campaign" needs): create a
  link (pick one of the org's `purpose: tracking` domains, type a slug,
  see a live `http://domain/t/slug` preview before submitting), a table of
  existing links with a copy button, pause/resume, and delete. Fully
  localized (`campaigns.json`'s new `trackingLinks` keys, en+ru) — this
  page sits inside an already-fully-Russian-localized campaign detail
  view, unlike the Domains page itself, which keeps its pre-existing,
  separately-scoped English-only UI (not migrated here — out of scope for
  this fix, a real, older, orthogonal gap).

## Known gaps (real, not silently faked)

- No "distinct values for dimension X"-equivalent for domains: the
  tracking-link form's domain picker is real (your own orgs' domains,
  filtered to `purpose: tracking`), but nothing suggests a slug — the
  operator types their own.
- The Domains page itself stays English-only (pre-existing, unrelated to
  this fix — matches `docs/frontend-i18n.md`'s already-documented
  migration backlog).
- No real DNS/SSL/registrar automation, as above — status fields are
  operator-declared, not verified.

## Verified

`gofmt`/`go vet`/`go build`/`go test -count=1 ./...` clean repo-wide,
including both new packages' repository tests against a real Postgres
(cross-org domain conflict, cross-org domain/campaign rejection on
tracking-link create, duplicate-slug conflict, invalid-slug validation).
Real browser pass against the actual reporting user's own account and
campaign (not a synthetic test org): added a real domain through the
Domains page, created a tracking link from the campaign's Overview tab,
copied it, and confirmed with a direct `curl` against the real
containerized tracker that it correctly 302-redirects to the campaign's
configured offer destination — the full, real click path, not just a
mocked-looking success screen. `tsc --noEmit`/`eslint` clean on
`apps/web`.

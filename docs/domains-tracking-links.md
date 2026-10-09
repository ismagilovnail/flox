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
- ~~No real DNS/SSL/registrar automation~~ — real for Namecheap
  (registrar) and Cloudflare (DNS), added later; see this doc's own
  "Registrar/DNS automation" section below. GoDaddy/Route53/
  cloudflare_registrar/unmanaged still have no adapter and stay
  operator-declared, unverified fields.

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

## Registrar/DNS automation (§30) — a later, separately-scoped phase

Picked off this doc's own "known gaps" list, after Settings >
Integrations (`docs/settings-integrations.md`) gave the org a real place
to connect Namecheap/Cloudflare credentials. Scoped with the user to all
three genuinely separable pieces at once: a read-only status sync, a
real (billable) renewal action, and a real (DNS-mutating) record-sync
action.

**Verify** (`POST /domains/{id}/verify`) checks whichever of
registrar=namecheap / dnsProvider=cloudflare a domain actually names,
against the org's own connected Settings > Integrations credential
(read through a new `domain.CredentialSource` interface —
`integrations.Service.CredentialsFor`, a trusted, Go-only method never
exposed over HTTP). Namecheap's real `domains.getInfo` command gives the
real expiry date; Cloudflare's real zone status + `/ssl/verification`
endpoint gives real activation/certificate state. The two checks are
genuinely independent — a domain can have one provider connected and the
other not, and Verify runs whichever half it can rather than failing the
whole call over the other half not being set up yet.

**A real bug this independence itself caught, not by inspection:** the
first implementation checked Namecheap, then Cloudflare, returning
immediately on the FIRST applicable-but-disconnected provider — so a
domain with registrar=namecheap (disconnected) and dnsProvider=cloudflare
(connected) never even attempted the real Cloudflare call. Found via
live curl verification (a domain in that exact shape returned a
Namecheap "connect first" error instead of running the real, connected
Cloudflare check), not caught by the original test suite, which only
ever exercised "both connected" and "neither connected." Fixed by
tracking "not connected" as a soft skip per provider rather than an
immediate return, only surfaced as the final error if literally nothing
ran; a regression test
(`TestVerifyRunsWhicheverHalfIsConnected`) covers the exact shape that
broke.

**Renew** (`POST /domains/{id}/renew`, registrar=namecheap only) calls
the real `namecheap.domains.renew` command — a genuine, billable action
against the org's connected account, gated behind `settings.write`
(Owner/Admin) and a frontend confirm dialog that says outright it will
charge the account and cannot be undone. A failed call (bad credentials,
insufficient balance, etc.) leaves the domain's stored `expiresAt`
untouched — nothing is recorded as renewed unless Namecheap actually
confirmed it.

**Sync DNS record** (`POST /domains/{id}/sync-dns`, dnsProvider=cloudflare
only) creates or updates a real, proxied CNAME record at Cloudflare
pointing the domain at `TRACKER_DNS_TARGET` — a new config value
(deliberately no default; refuses with a clear error while unset rather
than pointing a real customer's domain at a fabricated address). Also
gated behind `settings.write` and its own confirm dialog, which warns
that any existing traffic on the domain will move to FLOX. `EnsureRecord`
is a real upsert: it reads the existing record first and no-ops if it
already points at the right target, so repeat syncs don't generate
needless Cloudflare API writes.

Both `apps/internal/domain/namecheap` and `apps/internal/domain/
cloudflare` are new subpackages, structurally verified only (this
project has no live Namecheap/Cloudflare account) — the same "real HTTP
client, fake `httptest.Server` in tests" pattern `adaccount`'s own
adapters already established. Cloudflare's zone lookup derives the real
apex domain from a tracking subdomain via a naive last-two-labels
heuristic, documented as not handling multi-part public suffixes (e.g.
`.co.uk`) correctly — accepted for now since nothing here runs against a
real zone.

Verified: `go build/vet/test -count=1 ./...` clean, including new
`domain` package tests (both providers connected, only one connected —
the regression case above, neither connected, an unsupported-provider
domain like GoDaddy+Route53, Renew's registrar/years validation and its
"a failed renewal changes nothing" guarantee, SyncDNS's "refuses without
a configured target" guarantee) and two new adapter-level test suites
(Namecheap's real XML success/error shapes for both `getInfo` and
`renew`; Cloudflare's real JSON shapes for zone lookup, SSL verification,
and DNS record create/update/no-op). `tsc`/`eslint`/`vitest` clean.

Real, live, end-to-end verification against the actual running dev API —
including the regression above, found this way, not invented as a
hypothetical. With no integration connected, `verify`/`renew`/`sync-dns`
all returned clean, correctly-worded validation errors. To exercise the
connected path without a real Namecheap/Cloudflare account, a test
`org_integrations` row was inserted directly (AES-256-GCM encrypted with
the real dev `INTEGRATION_ENCRYPTION_KEY`, the same cipher
`apps/internal/integrations` itself produces) — confirmed `GET
/integrations` correctly reported it as connected, then `sync-dns` made
a **real live HTTP call to `api.cloudflare.com`** and got back
Cloudflare's own real `"Invalid access token"` error. After the fix
above, re-running the exact partial-connection shape (Namecheap
disconnected, Cloudflare connected) correctly ran the real Cloudflare
call instead of stopping on the Namecheap half. Real browser pass
(Claude-in-Chrome): created a real domain with registrar=Namecheap/
dnsProvider=Cloudflare, confirmed the real "Registrar & DNS automation"
card renders with live status/SSL/expiry/last-verified fields, clicked
Verify (a real 422 network call, confirmed via the network log) with no
console errors, opened both the Renew and Sync DNS confirm dialogs and
confirmed their real warning copy, and confirmed Sync DNS's real
network call. Test domain and org_integrations row deleted afterward.

## HTTPS for tracking domains (marketer audit 2026-10-06, #4)

Before this, `ssl` was set only by a Cloudflare check, so every other domain
showed `none` forever and every tracking link was `http://`. Now:

- **Issuance** is done by a TLS proxy in front of the tracker with on-demand
  certificates — `infra/Caddyfile.tracker`, see `docs/deployment.md`. Caddy
  issues a Let's Encrypt/ZeroSSL certificate the first time a domain is
  reached over HTTPS and renews it. FLOX itself holds no ACME code or keys.
- **Who gets a certificate**: Caddy asks `GET /internal/tls-ask?domain=` on
  the tracker (`internal/tlsask`) first; only a hostname on some
  organization's Domains page gets one. The Caddyfile hides `/internal/*`
  and `/metrics` from the public.
- **Status** comes from a real TLS connection, never from input:
  `internal/domain/tlsprobe` connects to `domain:443`, verifies the chain
  against the system roots and the hostname, and maps the result:

  | `ssl` | meaning |
  |---|---|
  | `issued` | valid certificate; `sslExpiresAt` is its expiry |
  | `pending` | port 443 answers but no certificate (not issued yet, or the proxy refused) |
  | `none` | nothing answers on 443 (DNS not pointed, no proxy) |
  | `error` | a certificate a browser would reject (expired, other name, untrusted); its expiry is still recorded |

  `sslError` holds the reason; `sslCheckedAt` when. A failed check keeps the
  last seen expiry.
- **Schedule**: the worker (`internal/sslcheck`) checks every minute for due
  domains — never checked, `issued` and older than 6 h, anything else older
  than 10 min — claiming them with `UPDATE … FOR UPDATE SKIP LOCKED`, so
  replicas do not double-probe. Under on-demand TLS the check's own
  connection triggers issuance, so a freshly pointed domain turns `issued`
  within one or two checks. `POST /domains/{id}/check-ssl` (the "Check SSL"
  button) runs it now.
- **Links**: a tracking link's `url` is `https://` once its domain is
  `issued` (`secure: true`), `http://` before — an https link to a domain
  without a certificate would fail in the visitor's browser. The campaign page
  marks such links with an open-lock icon linking to the domain.
- Probing rewrites `domains.ssl_checked_at` constantly, so migration 00056
  narrows the routing-cache NOTIFY trigger on `domains` to INSERT/DELETE/
  TRUNCATE and `UPDATE OF domain` — the only column the tracker reads.

Verified live: Caddy 2 with its local CA in front of the dev tracker —
`track.localhost` got a certificate on first connection, an unregistered
name got a TLS alert, `/internal/tls-ask` answered 404 through the proxy,
plain HTTP still served; the probe reported `error` (untrusted) with system
roots, `issued` with Caddy's root, and `pending` for the unregistered name.

## Domain ownership (tech audit 2026-10-06, T6)

Before: a domain tracked as soon as any organization typed it in —
`status` was client input, default `active` — and the name was unique
across all organizations, so whoever added someone else's domain first
held it (squatting) and the real owner could not add it at all.

Now:

- **A new domain is `pending`.** It becomes `active` only through
  `POST /domains/{id}/verify-ownership`: the API looks up the TXT record
  `_flox-verify.<domain>` and needs `flox-verify=<token>` in it. The token
  is per domain row (`verification_token`), so another organization's
  record for the same name proves nothing for this one. The domain page
  shows the record to add (copy buttons) and a "Check DNS" button; the
  form offers `active` only once verified, and the API refuses it
  otherwise (422).
- **Only `active` domains track and get certificates:**
  `routingstore.ResolveTrackingLink` and `tlsask` filter on it. The
  routing-cache trigger now fires on `status` changes too (it watched only
  `domain`), so verifying or deactivating takes effect in about a second.
- **Uniqueness is among active domains only** (`domains_active_domain_key`),
  plus once per organization (`domains_org_domain_key`). Any number of
  organizations may hold a name pending; a squatter blocks nobody. A name
  active elsewhere stays there — verifying it in a second organization is
  a 409, not a takeover (a transfer would write another tenant's data;
  that is a support action).
- **A rename is a new domain**: it resets verification and the token and
  takes an active domain back to pending — otherwise a verified domain
  could be renamed into someone else's and track on it at once.
- **Reserved names** (`.localhost`, `.test`, `.example`, `.invalid` — RFC
  2606/6761) are verified when added: nobody can own or squat them. The
  dev fixtures live on `track.localhost`.
- Domains already active when this shipped (migration 00062) were kept
  active and marked verified at their creation time: no production
  deployment existed yet.
- DNS is read through `domain.TXTResolver` (the system resolver in
  production), swappable like every other provider (CLAUDE.md #11).

Tests: two organizations add one name; the squatter cannot verify with
the owner's record; the owner verifies and gets it active; the squatter
then gets 409 even with its own record. Active refused unverified on
create and update; verified domains toggle freely; a rename resets
verification. Reserved names verify at once. The tracker 404s a link on a
pending domain; `tlsask` refuses a pending one. Live: a pending flip of
`track.localhost` took the demo link from 302 to 404 and back within a
second; the domain page showed the TXT record, the check reported the
missing record, `active` was disabled in the form.


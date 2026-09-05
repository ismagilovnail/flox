# Settings > Integrations (§30)

Real backend for the Integrations sub-panel — the last of Settings' own
frontend-mock pieces except Domains-adjacent registrar/DNS automation
itself, which stays deliberately out of scope (see below).

## Two halves, deliberately not one

Settings > Integrations lists six providers, but they split into two
architecturally different groups that never shared a home even in the
mock:

- **Facebook Ads / TikTok Ads / Google Ads** — real ad-spend cost
  integrations, connected **per traffic source**
  (`apps/internal/adaccount`, migration 00018), matching
  `trafficsource.CostIntegration`'s own singular "this source uses at
  most one integration" design. Facebook/TikTok already had a real
  connect/disconnect flow under Traffic Sources' own per-source form
  before this phase; Google Ads has none yet (a real candidate for its
  own later phase — see NEXT).
- **Namecheap / Cloudflare / Slack** — genuinely **org-level**
  credentials with no per-resource home to live under instead. This
  phase's own real backend: `apps/internal/integrations`.

Given that split, this panel was never going to be one uniform grid of
six equal "Connect" buttons. It renders two different card shapes: a
**read-only real aggregate** for the three ad platforms (a count of how
many of an org's traffic sources configured for that platform actually
have a connected ad account, sourced live from `adaccount`'s own data —
`AdPlatformSummary`, grouped by `traffic_sources.cost_integration` so a
future platform needs no code change here to appear) with a "Manage"
link to Traffic Sources, and a **real connect/disconnect/verify** card
for each of the three credential-based providers.

## Why manual credential paste, not OAuth (again)

Same environment constraint `adaccount`'s own doc already establishes:
no registered OAuth app with a public callback URL exists for Namecheap,
Cloudflare, or Google Ads in this environment. Slack is the one
exception worth calling out — an *incoming webhook* needs no registered
app at all (created once, by hand, in the target workspace), so its
adapter is real, live-testable infrastructure the moment an operator has
a URL to give it, not just structurally verified against a fake
transport like the other two.

## Schema: one row per (org, provider), encrypted at rest

`org_integrations` (migration 00033): `id`, `organization_id`,
`provider` (`namecheap`/`cloudflare`/`slack`), `encrypted_payload`,
`last_verified_at`, `last_error`, timestamps, `UNIQUE(organization_id,
provider)`. `encrypted_payload` is AES-256-GCM ciphertext of a flat JSON
object holding that provider's own credential fields — **encrypted, not
hashed**, since a real API call to verify/use the credential needs it
back in plaintext, unlike a password or API key which only ever need
one-way comparison (same reasoning `apps/internal/totp.EncryptSecret`'s
own doc comment gives for TOTP secrets).

`ad_account_connections` (migration 00018, predates this) stores its own
token in **plain text**, explicitly because no encryption infrastructure
existed yet at the time. This table doesn't repeat that gap — it uses
the real AES-256-GCM machinery `apps/internal/totp` already established,
via its own dedicated `INTEGRATION_ENCRYPTION_KEY` (deliberately a
separate key from `TOTP_ENCRYPTION_KEY`; rotating or compromising one
should never force rotating the other). Parsed once at startup
(`integrations.ParseKey`) with the same "fail loudly, never run with a
missing/well-known key" bar every other required secret in `main.go`
holds itself to.

## Connect requires success; Verify never deletes on failure

`Service.Connect` calls the provider's real `Verify` **before** writing
anything — a failed connect attempt leaves no row at all, the same
"no partial/broken intermediate state" bar the mock's own single
"Connect" action implied. `Service.Verify` (`POST /integrations/
{provider}/verify`) is a separate action for re-checking an
already-stored credential (e.g. a Cloudflare token later revoked
externally) — it never deletes the row on failure, it records the
failure as `last_error` so the row's status becomes `error` (visible,
with the real reason) rather than silently vanishing or staying falsely
`connected`. A subsequent successful re-verify clears it back to
`connected`.

## SSRF: Slack's webhook URL is operator-supplied

Unlike Namecheap/Cloudflare (fixed hostnames), Slack's webhook URL is
pasted by the operator and this package makes a real server-side POST to
it — the same SSRF shape `apps/internal/urlsafety` (§54) already exists
for (`network.Service`'s own `postback_url` field). `ValidateURL` rejects
an obviously-bad URL (a literal private/loopback/link-local/metadata IP)
before ever dialing it; the adapter's `http.Client` additionally uses
`urlsafety.SafeDialContext` as its `DialContext`, closing the
DNS-rebinding gap a save-time check alone can't (a hostname that
resolves to a public IP at save time could be repointed at a private IP
by the time the request actually connects).

## RBAC

Read (`GET /integrations`) is open to any authenticated member, matching
Organization/API Keys/Security's own precedent for this Settings panel.
Write (`connect`/`verify`/`disconnect`) requires `settings.write`
(Owner/Admin) — the same permission key those three already use, since
this is the same "org policy write" concern.

## Verified

`go build/vet/test -count=1 ./...` clean, including 12 new integration
tests against real Postgres (`Connect` requires a successful `Verify`
and writes nothing on failure, required-field validation, the full
connect → verify (success) → verify (failure, records `error` without
deleting) → verify (recovery) → disconnect lifecycle, `Verify`/
`Disconnect` 404 with no stored connection, an ad-platform provider name
is rejected as an integration provider, the real `AdPlatformSummary`
aggregate passes through unchanged, cross-org isolation) plus 7 adapter-
level tests against a fake `httptest.Server` per provider (Namecheap's
real XML success/error envelope, Cloudflare's real JSON success/error
envelope, Slack's real request/response handling, Slack's SSRF rejection
of a metadata-service URL).

Real, live, end-to-end verification over curl against the actual running
dev API — not curl-to-a-mock: signed up a real test org, confirmed `GET
/integrations` returns all three credential providers as
`not_connected` and an empty `adPlatforms` list; **a real live HTTP call
reached the real Cloudflare API** (`api.cloudflare.com`) with a
deliberately invalid token and got back Cloudflare's own real "Invalid
request headers" error, proving the adapter's real request shape against
the genuine endpoint, not just a fake transport; confirmed required-field
validation on Namecheap; confirmed `google_ads` (an ad-platform, not a
credential provider) is rejected by `Connect`; confirmed a Slack connect
attempt against `169.254.169.254` (a real cloud metadata address) is
rejected by `urlsafety` before any request is sent; confirmed `Verify`/
`Disconnect` 404 with nothing connected; connected a real Facebook Ads
ad account on a real traffic source via the existing `/traffic-sources/
{id}/connection` endpoint and confirmed `GET /integrations` immediately
reflected it (`{"provider":"facebook_ads","connectedCount":1,
"totalCount":1}`) — the real aggregate, not a cached or duplicated copy;
invited a real Buyer-role member and confirmed their read succeeds while
their connect attempt gets a real 403. All test mutations left in a
clean state (the failed connect attempts wrote nothing to clean up; the
test org itself was left, matching this session's own established
precedent — no delete-org endpoint exists).

Real browser pass (Claude-in-Chrome) against a disposable,
self-controlled test account from an earlier session's own browser-
verification work (not a real end user's account): the Integrations tab
renders all six real cards, including the genuine "Connected on 0 of 1
traffic source" aggregate for Facebook Ads (a real pre-existing traffic
source on that test org) and "Not configured on any traffic source yet"
for Google Ads/TikTok Ads; opened the Cloudflare connect dialog, typed a
fake token, submitted, and confirmed a real toast rendered the actual
error text returned by the live Cloudflare API call above — the failure
path works end to end through the real UI, not just over curl. No
console errors on any state. No test artifacts needed cleanup (the
failed connect wrote nothing).

## NEXT / deliberately not built here

- **Google Ads as a third real `adaccount.CostProvider` adapter**
  (traffic-source-scoped, matching Facebook/TikTok exactly) — its own
  separately-scoped phase. It needs a token-refresh step (Google Ads API
  access tokens are short-lived; an operator would paste a
  developer token + OAuth client id/secret + refresh token, minted once
  via Google's own OAuth playground since no registered public callback
  exists here) and a `traffic_sources.cost_integration` CHECK-constraint
  migration to add `'google_ads'` alongside `'facebook_ads'`/
  `'tiktok_ads'`. Once built, it needs zero change to this phase's own
  `AdPlatformSummary` aggregate query or this panel's frontend — both
  already read whatever `cost_integration` values actually exist.
- Real registrar/DNS automation for Domains (auto-renewal, DNS record
  sync, automatic SSL issuance) using the now-real Namecheap/Cloudflare
  credentials this phase stores — Domains' own `registrar`/`dns_provider`
  fields stay plain operator-declared enums (see
  `docs/domains-tracking-links.md`); wiring them to actually call these
  credentials is separately-scoped, real automation work, not a small
  add-on to either phase.
- No periodic background re-verification of a stored credential (no
  scheduler, unlike `costsync`'s own ad-spend sync) — `error` status only
  ever appears after an operator explicitly clicks "Verify."

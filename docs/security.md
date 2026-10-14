# Security hardening (§54, Phase 30)

Four independent defenses, each closing a gap that existed with zero
authentication/throttling/host-validation before this phase. None of them
touch `apps/tracker`'s redirect hot path (CLAUDE.md non-negotiable #9:
tracking p50 < 20ms / p95 < 50ms) except the postback-secret check below,
which only runs on `apps/tracker`'s separate `/postback/{networkId}`
handler — a low-volume, non-hot-path endpoint, not the click redirect.

## 1. Incoming postback authentication

Before this phase, `POST /postback/{networkId}` (`apps/tracker/postback.go`)
trusted the URL path's `{networkId}` alone — any client that observed or
guessed that ULID (a leaked URL, a referrer header, browser history, a log
line) could POST arbitrary conversion data as that network.

- `networks.postback_secret_hash` (migration `00021`, `DEFAULT ''`): a
  24-random-byte secret (`apps/internal/network/crypto.go`), SHA-256-hashed
  at rest — same one-way-hash-at-rest precedent as `sessions.token_hash`/
  `api_keys.key_hash`. The raw secret is returned exactly once: in the
  `POST /networks` response body (`CreateResult.PostbackSecret`) and in the
  `POST /networks/{id}/regenerate-postback-secret` response. Every other
  read of a `Network` (`List`/`Get`/`Update`/`Duplicate`) never carries it.
- The empty-string sentinel (a network created before this feature, or
  post-migration default) can never match any hashed input, so it safely
  rejects every postback without a separate nil check.
- `apps/tracker`'s `validPostbackSecret` hashes the request's `?secret=...`
  and compares against the stored hash with `subtle.ConstantTimeCompare` —
  an early-exit `==` would leak how many leading bytes matched through
  response timing.
- Regenerating (operator suspects a leak, or lost the original — it's never
  stored recoverably) overwrites the hash in place; the old secret stops
  working the instant that commits.
- Duplicating a network gets its own fresh secret, never the source
  network's, so two networks never share one credential.

## 2. CSRF: `RequireSameOrigin`

`SameSite=Lax` on the session cookie (`apps/internal/auth`) already blocks
the classic cross-site `<form>` POST, but not a cross-site `fetch`/XHR
issued with `credentials: "include"` — some browsers/configurations still
attach `Lax` cookies to those. `tenant.RequireSameOrigin` closes that gap:
reject any mutating request (`POST`/`PATCH`/`PUT`/`DELETE`) whose `Origin`
header is **present** and doesn't exactly match `APP_URL`.

- A request with **no** `Origin` header is allowed through deliberately —
  browsers always send `Origin` on a cross-origin fetch/XHR and on
  same-origin state-changing requests per the Fetch spec, so its absence
  means a non-browser client (curl, an operator script, a health check),
  never a browser silently omitting it. Rejecting those too would break
  legitimate API access for no CSRF benefit.
- `GET`/`HEAD`/`OPTIONS` are never checked — they don't change state.
- Mounted *before* (outside) session resolution in `apps/api/main.go`:
  rejecting a forged cross-origin mutation is cheaper than first looking up
  whose session it claims to be. `signup`/`login`/`accept-invite` are
  mutating but pre-session (no cookie exists yet to run tenant middleware
  against), so they get `RequireSameOrigin` alone — guarding against login
  CSRF (forcing a victim's browser to authenticate as an attacker-controlled
  account so the victim unknowingly saves real data into it).

## 3. Rate limiting

`apps/internal/ratelimit`: a fixed-window Redis counter — one Lua script
that increments and sets the window's expiry when the key has none, so a
key can never be left without a TTL (an endless 429; tech audit API1). **Fails open** on a Redis error —
logs a warning and allows the request — matching every other
Redis-optional path in this codebase (`conversion.RedisStore` falls through
to Postgres directly). A limiter that failed closed would turn a Redis
outage into a full API outage, strictly worse than temporarily losing
brute-force protection.

- General ceiling: 300 req/min, across every `apps/api` domain route
  (`apps/internal/httpserver`). Registered via `exemptPaths(...)` so
  `/health`, `/ready`, `/metrics` are never subject to it — an
  orchestrator's liveness probe or a Prometheus scrape should never trip a
  limit meant for abuse.
- Keyed per-organization for API-key-authenticated traffic, per-IP for
  everything else (unauthenticated requests and session-cookie browser
  traffic) — `tenant.PeekIdentity` (`apps/internal/tenant`) resolves
  identity into context, best-effort and without rejecting the request,
  mounted globally *before* the general limiter (`apps/internal/
  httpserver.New`'s own `identify` parameter); the limiter's key function
  (`apps/api/main.go`) reads that context. Closes the "one high-volume
  integration could exhaust the bucket shared with everything else behind
  its IP" gap this section used to flag as a known limitation, below —
  `NewMiddleware` (the real, enforcing per-route auth check) skips
  re-resolving when `PeekIdentity` already succeeded, so an authenticated
  request still costs exactly one Postgres/session round trip, not two.
- Auth-specific ceiling: 20 req/15min per IP on `/auth/login` and
  `/auth/signup` (`apps/api/main.go`) — deliberately much stricter, sized
  for the actual brute-force/credential-stuffing concern those two routes
  carry, not general dashboard traffic; stays IP-keyed (pre-session, no
  identity to key by yet).
- `/auth/*` does **not** fail open (tech audit API2): its middleware is
  `StrictMiddleware`, which on a Redis error counts in the API process
  instead — per replica, so N replicas allow up to N× the limit, but
  bounded. A Redis round trip that fails costs the request ~0.5 s of
  go-redis dial retries first.
- Per account, independent of Redis and of the IP: 10 failed sign-ins in
  15 minutes lock that email's sign-in until the window ends
  (`internal/auth/loginfailures.go`, Postgres `login_failures`, migration
  00063). A wrong second-factor code counts like a wrong password, and the
  count is cleared only when a session is issued — so knowing the password
  does not buy unlimited code guesses. Keyed by sha256 of the lowercased
  email, not the user: an email with no account locks the same way, so
  the 429 reveals nothing about which accounts exist. While locked even
  the right password gets 429. The trade-off: anyone can keep an
  account's sign-in locked by failing on purpose (20 tries per 15 min per
  IP are enough); existing sessions are unaffected.
- **Not** applied to `apps/tracker`'s redirect hot path — a synchronous
  Redis round trip on every click would blow the p50/p95 budget outright.
  Click-volume protection there is an infrastructure concern (CDN/load
  balancer), not an in-process limiter.
- `apps/tracker`'s `/postback/{networkId}` limits **refusals**, not
  requests (tech audit P4): 30 postbacks refused as an unknown network or
  a wrong secret within 10 minutes block that IP for the rest of the
  window, answered 429 before any Postgres lookup
  (`ratelimit.Failures`). Accepted postbacks, duplicates and 400s never
  count, so a network's nightly replay of any size is never limited. A
  network that keeps sending from an old postback URL beside its good one
  gets its good postbacks 429'd too once over the limit — 429 is
  retryable, and the refusals show up in `flox_postback_incoming_total`.
  Counts in the tracker process while Redis fails, like `/auth/*`. Volume
  from a valid network and secret is still the edge's concern.
- The "per IP" case in every ceiling above is `apps/internal/
  clientip.Resolver`'s resolved address (§5 below), not a raw header
  read — see that section for why that distinction is load-bearing.

## 4. SSRF: `apps/internal/urlsafety`

The one outbound call in this codebase to an operator-supplied host:
`apps/internal/postback.Deliverer`'s delivery request to a network's
`postback_url` (macro-substituted, `apps/internal/macro`). Two layers:

- `ValidateURL` — a cheap, save-time check in `network.Service`
  (create/update): scheme must be `http`/`https`, host must not be a
  *literal* IP in a forbidden range.
- `SafeDialContext` — the actual, authoritative defense, set as
  `apps/worker`'s postback `http.Client`'s `Transport.DialContext`.
  `ValidateURL` alone can't be the real protection: a hostname resolving to
  a public IP at save time can be repointed at a private/metadata address
  by delivery time (DNS rebinding) — a save-time check has no way to catch
  that. `SafeDialContext` resolves the host and validates the IP at the
  moment of connection, then dials that *exact validated IP* (never the
  hostname again — a second lookup could return a different, rebound
  address, reopening the gap).
- Forbidden ranges (`isForbidden`): loopback, RFC 1918/4193 private
  (`net.IP.IsPrivate`), link-local unicast/multicast (including
  `169.254.169.254`, the cloud-metadata address every major provider
  uses), and the unspecified address.
- `apps/internal/adaccount/facebookads` and `.../tiktokads` never take an
  operator-supplied host (only a hardcoded API hostname), so they aren't an
  SSRF vector and don't use this package.

## 5. Trusted-proxy IP resolution & pending-login lockout (audit findings #2, #3)

Before this fix, every client-IP read in the tree —
`apps/internal/ratelimit`'s own `ClientIP`, `apps/tracker`'s own `clientIP`
helper, and chi's `middleware.RealIP` mounted in front of both — took
`X-Forwarded-For`'s **leftmost** entry unconditionally. That entry is
exactly the one the client itself writes, so all three were
attacker-controlled: rotating the header defeated the §3 auth rate limit
outright, made the org IP allowlist (Settings > Security) decorative, and
let a tracker visitor spoof the country/ASN that feed routing filters and
choose its own weighted-flow arm (§38).

`apps/internal/clientip.Resolver` replaces all three. It trusts
`X-Forwarded-For` only from a peer in `TRUSTED_PROXIES` (`.env`; default
loopback + RFC 1918/4193 private ranges, covering "no real proxy
deployed yet" safely since a public peer is never in that set), then walks
the header right-to-left for the first non-trusted entry — the header
grows left to right as each hop appends its upstream, so the rightmost
untrusted entry is the closest verifiable claim to the real client. `chi`'s
own `middleware.RealIP` is deprecated upstream for the identical
unconditional-trust reason (GHSA-3fxj-6jh8-hvhx, GHSA-rjr7-jggh-pgcp,
GHSA-9g5q-2w5x-hmxf) and is no longer mounted in either binary.

Separately, `VerifyPendingLogin` (`apps/internal/auth/service.go`) used to
leave a wrong TOTP/backup-code attempt's pending row untouched, so its
5-minute TTL allowed unlimited guesses against one token. A wrong code now
deletes the pending row immediately (`DeletePendingTOTPLogin`), forcing a
fresh `Login` call — the same "wrong attempt costs the whole token" trade
`VerifyEmail`/invite tokens already make elsewhere in this file.

## 6. API key authentication (audit finding #4, closed)

`apps/internal/apikey`'s `Create`/`List`/`Revoke` were real, hashed
credential bookkeeping, but nothing ever authenticated a request with
one — no `Authorization` middleware existed anywhere, and the
`read`/`write`/`admin` scope stored per key was read by zero handlers.
Confirmed and fixed: `tenant.NewMiddleware` (the one choke point every
tenant-scoped route already shares) now checks `Authorization: Bearer
flx_live_...` before falling back to the session cookie, calling
`apikey.Service.Authenticate` (hash lookup, `revoked_at IS NULL` check,
best-effort `last_used_at` stamp). `RequirePermission` evaluates a
key's scope as a coarse tier — `read`/`write`(minus `team.write` and
`settings.write`)/`admin` — by permission-key suffix, no DB query.
Full detail, including the live end-to-end verification against a real
running API, is in `docs/auth.md`'s "API key authentication" section.

## 6a. API key rotation (§54, Phase 30.18)

`POST /api-keys/{id}/rotate` (`settings.write`) mints the key's successor —
same name and scope, a new secret shown once — and gives the old key
`expires_at = now() + 24h` and `replaced_by_id` (migration `00072`). Both
secrets authenticate during those 24 hours, so an integration switches
without downtime; afterwards the old one is refused ("api key expired") and
leaves the list. A key that already has a successor cannot be rotated again
(409): rotate the replacement. Revoking still stops a key at once, also
during its grace period. Settings > API keys has a Rotate button; a replaced
key shows when it stops working.

## 7. Security headers (tech audit S2)

| | `apps/web` (`next.config.ts`) | `apps/api` (`httpserver.securityHeaders`) |
|---|---|---|
| `Content-Security-Policy` | `frame-ancestors 'none'; object-src 'none'` | `default-src 'none'; frame-ancestors 'none'` |
| `X-Frame-Options` | `DENY` | `DENY` |
| `X-Content-Type-Options` | `nosniff` | `nosniff` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | `no-referrer` |
| `Permissions-Policy` | camera, microphone, geolocation off | — |
| `Strict-Transport-Security` | `max-age=31536000` in the built app | same, when `NODE_ENV` ≠ `development` (the Secure-cookie gate: TLS in front) |

The audit's concrete risk was clickjacking — another site framing the
app over a destructive button; `frame-ancestors 'none'` closes it
(verified: a page on another origin gets a blocked frame).

**Not done: a script policy.** The landing editor previews a landing in
an `<iframe srcDoc>`, and a srcdoc document inherits the page's CSP: a
`script-src`/`base-uri` policy would block the landing's own scripts and
its `<base href>` to the asset CDN. Verified that the policy above leaves
such a preview working (script runs, base applies). A nonce-based
`script-src` (Next.js `proxy.ts`) needs the preview moved to its own URL
first. `apps/tracker` is unchanged: it serves visitors' landings and PWA
pages, which set their own headers (`nosniff`), and must stay embeddable
where a campaign needs it.

## Known limitations

- No CAPTCHA/device-fingerprinting on `/auth/login`/`/auth/signup` — rate
  limiting is the only brute-force defense.
- `TRUSTED_PROXIES` defaults to private ranges only; a deployment behind a
  real reverse proxy, load balancer, or CDN must set it to that proxy's
  actual address range, or `X-Forwarded-For` stays ignored and every
  client behind it shares one rate-limit bucket.
- `urlsafety` validates IPv4/IPv6 private/loopback/link-local/metadata
  ranges; it does not maintain a cloud-provider-specific IP allowlist
  beyond the generic `169.254.0.0/16` metadata range.
- The request rate limit is per-IP — a distributed attacker (many source
  IPs) is not throttled by it; that's an infrastructure/WAF concern. One
  account's password is still bounded by the per-account limit (§3);
  spraying one password across many accounts is not.
- No CSP/security-header hardening (`X-Frame-Options`, `Content-Security-
  Policy`, ...) — out of scope for this phase, not requested.
- ~~API keys authenticate nothing (audit finding #4)~~ — closed, see §6
  above.
- ~~API-key-authenticated requests share the general rate limit's
  IP-keyed bucket with everything else behind the same address~~ —
  closed, see §3 above (`tenant.PeekIdentity` + a per-organization key
  for that case).
- `govulncheck ./...` reports one permanent, accepted advisory:
  `GO-2026-5932` (`golang.org/x/crypto/openpgp` is unmaintained and
  unsafe by design). `Fixed in: N/A` — upstream has abandoned the
  package, there is no newer version that resolves it. Not a live risk:
  this codebase's only `x/crypto` import is `bcrypt`
  (`internal/auth/crypto.go`), a different subpackage `openpgp` doesn't
  touch, and `govulncheck` independently confirms 0 reachable
  vulnerabilities. The two `x/crypto/ssh` advisories that were flagged
  alongside it (`GO-2026-6355`, `GO-2026-6354`) were fixed by bumping to
  v0.56.0 — this is the one that can't be bumped away, and won't be
  until/unless upstream either fixes or removes `openpgp`. Re-flagging
  this in a future audit would be a false positive, not a regression.
  Fixing this needs auth resolution to happen before the general
  limiter runs, which is a bigger restructuring than §6's phase scoped;
  noted for whoever picks up API traffic at real volume.

## 8. Audit trail (§54, Phase 30.18)

`internal/audit.Middleware` runs inside the tenant middleware
(`apps/api/main.go`'s `tenantMiddleware`), so every tenant route is covered,
including routes added later. It writes one `audit_logs` row per successful
(2xx) POST/PUT/PATCH/DELETE:

- **who:** `actor_user_id` is the signed-in user, or for an API key its
  creator, with `metadata.apiKeyId` naming the key;
- **what:** a named action when the handler or service gave one through
  `audit.Annotate` (`api_key.created/rotated/revoked`,
  `integration.connected/disconnected`, `ad_account.connected/disconnected`,
  `network.postback_secret_regenerated`, the `team.*` and
  `settings.updated` actions); otherwise `"METHOD /route"`, the route
  pattern without `/v1`;
- **which:** the entity of the last URL parameter, or the id of a resource
  a 201 response created;
- `metadata` = method, route, status (and `apiKeyId`).

Request and response bodies are never stored: they carry the secrets §54
forbids logging (API keys, postback secrets, ad-platform tokens). Failed
requests and reads are not recorded; neither are the POST-shaped reads in
`auditSkip` (analytics queries, routing simulate, push audience, marking
notifications read). `TestAuditTrail` (apps/api) checks that every
`auditSkip` entry is a real route, and runs a session write and an API-key
write end to end. The insert is best-effort and runs after the response
has been written: a failed write is logged, never turned into an error.
One gap remains: a change whose request then fails, such as a service error
after the commit, leaves no row.

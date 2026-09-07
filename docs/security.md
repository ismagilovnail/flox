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

`apps/internal/ratelimit`: a fixed-window Redis counter (`INCR`, `EXPIRE`
once on the window's first request). **Fails open** on a Redis error —
logs a warning and allows the request — matching every other
Redis-optional path in this codebase (`conversion.RedisStore` falls through
to Postgres directly). A limiter that failed closed would turn a Redis
outage into a full API outage, strictly worse than temporarily losing
brute-force protection.

- General ceiling: 300 req/min per IP, across every `apps/api` domain
  route (`apps/internal/httpserver`). Registered via `exemptPaths(...)` so
  `/health`, `/ready`, `/metrics` are never subject to it — an
  orchestrator's liveness probe or a Prometheus scrape should never trip a
  limit meant for abuse.
- Auth-specific ceiling: 20 req/15min per IP on `/auth/login` and
  `/auth/signup` (`apps/api/main.go`) — deliberately much stricter, sized
  for the actual brute-force/credential-stuffing concern those two routes
  carry, not general dashboard traffic.
- **Not** applied to `apps/tracker`'s redirect hot path — a synchronous
  Redis round trip on every click would blow the p50/p95 budget outright.
  Click-volume protection there is an infrastructure concern (CDN/load
  balancer), not an in-process limiter.
- The "per IP" in both ceilings is `apps/internal/clientip.Resolver`'s
  resolved address (§5 below), not a raw header read — see that section
  for why that distinction is load-bearing.

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
- Rate limiting is per-IP only — a distributed attacker (many source IPs)
  is not throttled by this layer; that's an infrastructure/WAF concern.
- No CSP/security-header hardening (`X-Frame-Options`, `Content-Security-
  Policy`, ...) — out of scope for this phase, not requested.
- ~~API keys authenticate nothing (audit finding #4)~~ — closed, see §6
  above.
- API-key-authenticated requests share the general rate limit's
  IP-keyed bucket with everything else behind the same address (§3) —
  a single high-volume legitimate integration server could exhaust it.
  Fixing this needs auth resolution to happen before the general
  limiter runs, which is a bigger restructuring than §6's phase scoped;
  noted for whoever picks up API traffic at real volume.

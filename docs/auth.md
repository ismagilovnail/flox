# Auth / Organizations / RBAC (§52, Phase 28)

Split into two phases (confirmed via `AskUserQuestion` before starting):
**Phase 28A** builds the real backend — authentication, sessions,
organizations, memberships, and role-based authorization, replacing
`apps/internal/tenant`'s original `X-Organization-Id`-header stand-in.
**Phase 28B** (second half of this doc) wires `apps/web`'s existing mock
Team UI, and a newly-built login/signup/accept-invite UI, to that API.

## Scope decisions (all confirmed via `AskUserQuestion`)

- **Server-side sessions in an HTTP-only cookie, not JWTs.** Trivially
  revocable (`DELETE` the row) with no rotation/denylist machinery to get
  wrong — see migration `00020`'s own comment on the `sessions` table.
- **Inviting a team member generates a shareable accept-invite link**, not
  a real sent email. This project has no SMTP/email-provider integration
  anywhere (checked before deciding) — the same situation Ad Account
  Connections hit with OAuth (no registered app to redirect through) and
  resolved the same way: build the real mechanism up to the boundary that
  needs infrastructure this environment doesn't have, rather than fake an
  "email sent" success state.
- **RBAC permission enforcement (`RequirePermission`) is wired onto this
  package's own team/membership endpoints only.** §52 also calls for
  enforcing authorization server-side across the whole product, but
  retrofitting it onto every pre-existing domain route (campaigns, offers,
  sources, pixels, stream sets, cost, ...) means touching ~15 already-
  shipped packages' routes — confirmed as an explicit, separate follow-up
  phase, not a silent expansion of this one. Tenant isolation (§36-
  TENANCY: no org ever sees another org's data) is unaffected either way —
  it was already enforced everywhere via org-scoped repositories, and is
  orthogonal to role-based permission checks *within* one org.

## What already existed (migration 00001, prior phases)

`organizations`, `users` (no password field yet), `roles` (the six fixed
roles), `permissions` (the ten fixed permission keys), and `memberships`
(with `status` already modeling `active`/`invited`/`suspended`) were all
created as schema-only groundwork before this phase, explicitly deferring
"auth fields (password hash, sessions, MFA)" and the role→permission
mapping to "Phase 28's job." `apps/web/src/lib/mock/team.ts`'s
`ROLE_PERMISSIONS` table has been this project's reference vocabulary for
§52 since Phase 14 — migration `00020`'s seed data matches it exactly,
cell for cell (verified via a `psql` query during this phase, see
Verified below).

## Migration `00020_auth_sessions_rbac.sql`

- `users.password_hash text NOT NULL DEFAULT ''` — an empty string can
  never bcrypt-match a real password, so it doubles as the sentinel for "no
  password set yet" (a shell user created by an invite, before they've
  accepted it) without a separate nullability check at every read site.
- `sessions` (`id`, `user_id`, `organization_id`, `token_hash` unique,
  `expires_at`) — `token_hash`, never the raw bearer token, same one-way-
  hash-at-rest precedent as `api_keys.key_hash` (migration `00010`).
  Scoped to exactly one org: `tenant.OrgID(ctx)` resolves to a single org
  per request, and nothing in `apps/web` exposes an org switcher today — a
  user in more than one org simply holds a separate session per org
  (chosen at login time, see Login below).
- `memberships.invite_token_hash` / `invite_token_expires_at` — an invite
  lives on the membership row itself, not a separate table: `status =
  'invited'` was already a first-class state (migration `00001`), and an
  accepted invite is just that same row transitioning to `active`.
- `role_permissions (role_id, permission_id)` — seeded by joining on
  `roles.key`/`permissions.key` text values (not hardcoded ids), matching
  `ROLE_PERMISSIONS` exactly: Owner/Admin get every permission; Manager,
  Buyer, Analyst, Viewer get the same subsets the mock UI has always
  described.

## `apps/internal/auth` (new package)

`handler.go`/`team_handler.go` → `service.go` → `repository.go` →
`model.go`/`crypto.go`, same layering as every other domain.

### Password + token handling (`crypto.go`)

- Passwords: `bcrypt`, default cost. `checkPassword` short-circuits on an
  empty hash (the shell-user sentinel) without paying bcrypt's comparison
  cost for an account that can't possibly log in yet.
- Session/invite tokens: 32 bytes of `crypto/rand`, base64url-encoded for
  the bearer value; only its SHA-256 hex hash is ever persisted. Both use
  the exact same shape — a session cookie and an invite link are both
  "possession of this opaque string proves who you are for one specific
  purpose," so one `newBearerToken()` helper serves both.
- `dummyPasswordHash`: a bcrypt hash computed once at process start purely
  so `Login` always pays bcrypt's comparison cost, whether or not the
  email matched a real account — without it, a nonexistent-email login
  would return measurably faster than a wrong-password one, letting an
  attacker enumerate registered emails by timing alone.

### Signup

`POST /auth/signup` — one transaction: a brand new organization, a brand
new user, and an `Owner` membership binding them, all three or none.
Validates org name/name/email shape/password length (≥ 8) before ever
touching the database. Returns the same `authResponse` shape as login and
sets the session cookie.

### Login

`POST /auth/login` — looks up the user by email (case-insensitive),
checks the password (against the real hash if found, the dummy hash if
not — see above), then resolves *which* organization the session should
be scoped to:

- Zero active memberships → 401 (an invited-but-not-yet-accepted or
  suspended-everywhere account can't log in at all).
- Exactly one active membership → used automatically.
- More than one (a user invited into a second org) → the request must
  include `organizationId` to disambiguate, or gets a 422 naming exactly
  that field. Nothing in `apps/web` exposes an org switcher yet, so this
  case is untested by any UI today, but the schema makes it possible
  (`memberships` is many-to-many), so `Login` handles it rather than
  picking an org arbitrarily.

### Sessions and `tenant.NewMiddleware`

`apps/internal/tenant` changed from a package-level `Middleware` function
value (reading `X-Organization-Id` off the request) to `NewMiddleware(
resolver SessionResolver, logger *slog.Logger) func(http.Handler)
http.Handler`, constructed once in `apps/api/main.go` with `authSvc`
(which implements `SessionResolver.ResolveSession` by hashing the cookie's
token and delegating to the repository). `tenant.OrgID(ctx)`/the brand new
`tenant.UserID(ctx)` are otherwise unchanged — every existing handler
across all ~15 other domain packages needed zero code changes; only
`apps/api/main.go`'s 17 `r.Use(tenant.Middleware)` call sites became
`r.Use(tenantMiddleware)`. `tenant` does not import `auth` (avoiding an
import cycle, since `auth` imports `tenant` for `OrgID`/`UserID` in its own
handlers) — `SessionResolver` is declared in `tenant`, at the point of use,
and `main.go` wires the concrete `*auth.Service` into it.

`Repository.ResolveSession` validates the token and, in the same query,
bumps the matching membership's `last_active_at` — the Team UI's member
list has always expected real data there (`apps/web/src/lib/mock/team.ts`'s
`TeamMember.lastActiveAt`). Requiring `status = 'active'` in that same
join means **suspending or removing a member takes effect on their very
next request**: an existing session cookie for a suspended/removed
membership resolves to zero rows, not a stale success. `UpdateMembership`/
`DeleteMembership` also proactively delete that user's session rows for
the org (belt-and-suspenders — the join check alone is already correct,
this just also cleans up the now-dead rows instead of leaving them to
expire naturally).

### `GET /auth/me`

Returns `{user, organization, role, permissions}` — the full permission
list for the caller's role, so Phase 28B's frontend can gate UI
(`RolesPermissionsPanel` already documents this exact table). This is
UX-only, per §52 ("frontend permissions are only UX") — every mutating
endpoint still checks permissions server-side independently.

### Invite / accept-invite

- `POST /team/members/invite` (`team.write`) — validates name/email/role
  (rejects `Owner` — there is exactly one per org, created at signup,
  never assigned afterward — matching
  `invite-member-sheet.tsx`'s own `INVITABLE_ROLES`), finds-or-creates a
  "shell" user by email (`password_hash` left at its `''` sentinel), and
  creates an `invited` membership with a fresh token. Returns
  `{inviteUrl}` — `{APP_URL}/accept-invite?token=...` — a page Phase 28B
  builds.
- `GET /auth/invites/{token}` — public, token-guarded preview
  (`{organizationName, email, role}`) so that future accept-invite page
  can render "You've been invited to join X as Y" before the invitee sets
  a password.
- `POST /auth/accept-invite` — validates the token (must be `invited` and
  unexpired), sets the user's real password + name, flips the membership
  to `active`, and logs the invitee straight in (same session-cookie
  response shape as signup/login). The token cannot be replayed —
  accepting twice fails the second time (verified by
  `TestInviteAcceptAndPermissions`).
- `POST /team/members/{id}/resend-invite` (`team.write`) — regenerates the
  token and expiry (also bumping `invited_at`, matching the mock
  `resendInvite`'s own behavior) and **invalidates the previous token**
  (verified by `TestResendInviteInvalidatesThePreviousToken`).

### Membership management

`GET /team/members` / `PATCH /team/members/{id}` (role and/or status,
PATCH-partial like every other domain's `UpdateInput`) / `DELETE
/team/members/{id}` — all `team.write` except the read. Every mutation
refuses to touch a membership whose role is `Owner` (matching
`member-row-actions.tsx`'s own "no actions rendered for the Owner row" —
enforced server-side too, since frontend permissions are only UX) and
`role` can never be set to `Owner` via this endpoint either.

### Activity log

`GET /team/activity` (`team.read`) reads `audit_logs` (migration `00010`,
existed but was completely unused until this phase). Populated only by
this package's own membership actions (`team.invited`,
`team.invite_accepted`, `team.role_changed`, `team.suspended`,
`team.reactivated`, `team.removed`) — sweeping audit-log writes into every
other domain's write path is out of scope here for the same reason the
RBAC sweep is (see Scope decisions above).

## Cookie details

`Path=/`, `HttpOnly`, `SameSite=Lax`. `Secure` is `cfg.Env != "development"`
— a `Secure` cookie is dropped by browsers over plain `http`, and this
project's dev stack runs `apps/api` on `http://localhost` with no TLS
termination in front of it. `SameSite=Lax` rather than `None`: SameSite is
defined by scheme + registrable domain, not port, so `apps/web` (`:3000`)
and `apps/api` (`:8080`/`:18080`) are different *origins* but the same
*site* in local dev, and would still be in any same-domain production
deployment — `Lax` already covers both. A genuinely cross-site deployment
(different registrable domains) would need `SameSite=None; Secure`
instead; noted as a known limitation rather than built speculatively.

`apps/internal/httpserver`'s CORS config flipped `AllowCredentials` to
`true` (required for the cookie to travel on `apps/web`'s cross-origin
`fetch` calls) and dropped `X-Organization-Id` from `AllowedHeaders` (no
route accepts it anymore). `AllowedOrigins` stays a single explicit origin
(`cfg.AppURL`) — the fetch spec forbids combining a wildcard origin with
credentialed requests anyway.

## Known limitations (explicitly out of scope for this phase)

- **RBAC enforcement on pre-existing domain routes** (campaigns, offers,
  sources, pixels, stream sets, cost, ...): none of them check permissions
  yet — any authenticated member of an org can call any of their
  endpoints regardless of role. Confirmed as a separate follow-up phase.
- **No email delivery.** Invites produce a link an inviter must send
  manually (Slack, copy-paste, whatever) — there is no SMTP/provider
  integration to send it automatically.
- **No password reset / email verification / MFA.** Not requested; §52
  doesn't call for them either.
- **No API-key authentication.** `api_keys` (migration `00010`) still
  exists and is still completely unused — this phase only built session-
  cookie auth, matching what the user's original request ("Auth /
  Organizations / RBAC") and §52's own "sessions" bullet named.
- **No org switcher.** A user who accepts an invite into a second org
  (`TestLoginRequiresOrganizationIDWhenMultipleActiveMemberships`) can log
  into either org by passing `organizationId` at login, but there's no UI
  anywhere (mock or real) for switching orgs mid-session — logging out and
  back in against the other org is the only path today.
- **No expired-session cleanup job.** Expired `sessions` rows are simply
  never resolved again; nothing deletes them. Harmless (no correctness
  impact) but would accumulate indefinitely — a candidate for a future
  scheduled job, not built here since nothing requested it.

## Verified

Backend: `gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` all
green across the **entire** repo (all ~15 pre-existing domain packages'
own tests still pass unchanged — they call services/repositories directly
in Go, never through HTTP, so the `tenant.Middleware` → `tenant.NewMiddleware`
change touched zero of them). New tests in `internal/auth` (20 cases, all
against real Postgres): signup (creates org+owner+session, rejects
duplicate email, validates input), login (correct/wrong/unknown
credentials, case-insensitive email, single vs. multiple active
memberships), logout, session resolution (garbage token rejected), `Me`
(role + permission list), invite → preview → accept → login (including
"cannot replay an accepted token" and the Analyst role's actual permission
set), invite validation (rejects `Owner`, rejects a duplicate member),
resend-invite (old token invalidated), membership update (role change,
suspend revokes the session immediately, activity log entries), Owner
protection (cannot change own role or remove self), member removal
(revokes access, further mutations 404), cross-tenant isolation (org B
cannot update/remove/see org A's memberships or activity — required by
§36-TENANCY's own DoD), and the `RequirePermission` middleware (200 for a
role holding the permission, 403 for one that doesn't).

Full manual pass against the real running `apps/api` (Postgres/
ClickHouse/Redis dev stack), via `curl` with a cookie jar (no frontend
exists yet to click through — Phase 28B's job):

- Confirmed the *old* mechanism is gone: `GET /campaigns` with only
  `X-Organization-Id` (no session cookie) now correctly 401s.
- Signup → real `Set-Cookie: flox_session=...; HttpOnly; SameSite=Lax` →
  `GET /auth/me` succeeds → `GET /campaigns` (an untouched, pre-existing
  domain) succeeds through the exact same cookie, with no code changes to
  `campaign`'s own package at all.
- `GET /team/members` showed just the Owner; `POST /team/members/invite`
  returned a real `inviteUrl`; the public `GET /auth/invites/{token}`
  preview returned the correct org/email/role; `POST /auth/accept-invite`
  logged the invitee straight in with their own session cookie.
- The invited Viewer's `GET /auth/me` correctly showed only
  `["analytics.read","campaign.read"]`; their own `POST
  /team/members/invite` attempt correctly 403'd.
- The Owner suspended the Viewer (`PATCH .../members/{id}` with
  `status:"suspended"`) — the Viewer's *existing* session immediately
  401'd on its very next request, no re-login needed to observe the
  revocation. Reactivating restored nothing retroactively (a fresh
  login would be required) — expected, matching "suspension revokes the
  session" as designed.
- The Owner's own attempt to `DELETE` their own membership correctly
  422'd ("cannot remove the organization owner").
- `GET /team/activity` showed all five real audit-log entries
  (`team.invited`, `team.invite_accepted`, `team.suspended`,
  `team.reactivated`, `team.removed`) in the correct order with the
  correct actor names.
- `POST /auth/logout` cleared the cookie (`Max-Age=0`) and the same
  cookie's subsequent `GET /auth/me` correctly 401'd.
- Cleanup: deleted the `Curl Test Org` fixture (cascades to its user,
  membership, and any sessions).

No frontend changes this phase (Phase 28B's job) — `apps/web` still uses
`NEXT_PUBLIC_DEV_ORG_ID` + the mock Team store; neither was touched.

# Phase 28B: frontend integration

Wires `apps/web` to Phase 28A's API: a new login/signup/accept-invite UI
(none of it existed before, not even as a mock), route protection, and
the existing mock Team page switched onto the real `/team/*` endpoints.
`NEXT_PUBLIC_DEV_ORG_ID` is gone entirely — every request is scoped by
session cookie now, not a developer-set org id.

## `lib/api/client.ts`

`apiFetch` sends `credentials: "include"` instead of an
`X-Organization-Id` header, and no longer throws `MissingDevOrgError`
(deleted along with the env var). `apps/internal/httpserver`'s CORS
config flipped `AllowCredentials` to `true` with a single explicit
`AllowedOrigins` entry (`cfg.AppURL`) — required for the session cookie
to travel on `apps/web`'s cross-origin `fetch` calls; the fetch spec
forbids combining a wildcard origin with credentialed requests.

## New `(auth)` route group

`(auth)/layout.tsx` is a bare centered-card shell — no sidebar/topbar,
nothing to navigate to before a session exists. Three pages:
`/login`, `/signup`, `/accept-invite` (reads `?token=` via `PageProps`'
`searchParams`, a Server Component prop, rather than the client-only
`useSearchParams` — avoids that hook's own Suspense-boundary requirement
entirely). Each is a thin `page.tsx` wrapping a `features/auth/*-form.tsx`
(React Hook Form + Zod, i18n via a new `auth` namespace — the one new
UI surface this phase adds, so it follows this project's current i18n
convention; the pre-existing Team feature files, which predate that
convention and were never migrated, were left exactly as they already
were, hardcoded English, rather than partially retrofitting i18n into
only the parts this phase happened to touch).

**A real bug caught during manual testing, not code review:**
`AcceptInviteForm` originally pre-filled the name field via
`useForm({ values: {...} })`. `useForm`'s `values` option *re-applies on
every render*, and the object literal passed to it
(`{ name: preview.data?.email.split("@")[0] ?? "", password: "" }`) is a
new reference every render — so every keystroke into the password field
triggered a re-render, which reset the whole form back to that literal,
silently wiping the password the invitee had just typed. The symptom in
the browser was exactly "the submit button does nothing" — the form was
resetting itself out from under the user faster than they could finish
typing. Fixed with `defaultValues: { name: "", password: "" }` plus a
one-time `useEffect`/`useRef` guard that calls `form.setValue("name",
...)` exactly once when the invite preview first loads. Lesson for any
future form here: `values` is for a form that should stay in sync with
external state for its whole lifetime (rare); a one-time prefill from
async data almost always wants `defaultValues` + an effect instead.

## Route protection: `proxy.ts`, not `middleware.ts`

This Next.js version (16.3) renamed the `middleware.js` file convention
to `proxy.js` — `middleware.js` still works but is deprecated. `src/
proxy.ts` checks only whether the `flox_session` cookie is *present*
(`SESSION_COOKIE_NAME`, manually kept in sync with
`apps/internal/tenant.CookieName` — no shared constant between the Go and
TypeScript codebases), redirecting an unauthenticated visitor to
`/login` for every path except a small `PUBLIC_PATHS` allowlist (`/`,
`/login`, `/signup`, `/accept-invite`, `/style-guide`), and redirecting
an already-authenticated visitor away from `/login`/`/signup`. This is a
UX convenience only, stated explicitly in its own doc comment — it never
validates the cookie, since that would mean an API round trip on every
navigation. The real enforcement boundary is unchanged: apps/api.

`components/shell/auth-guard.tsx` is the defense-in-depth layer behind
it, wrapping `(app)/layout.tsx`'s children: it's what actually calls
`GET /auth/me`, so a cookie that's *present but expired/revoked* (past
its 30-day expiry, or a membership suspended/removed mid-session) is
caught here — `useMe()`'s `data === null` triggers a client-side redirect
to `/login` instead of leaving the visitor on an app shell whose every
fetch would otherwise silently 401.

## Team page: mock → real API

New `lib/api/team.ts` (`Membership`, `ActivityEntry` — mirrors
`apps/internal/auth`'s JSON exactly) and `hooks/use-team.ts` replace
`stores/team.ts`'s Zustand store *for the Team page only*.
`lib/mock/team.ts`'s `Role`/`ROLES`/`PERMISSIONS`/`ROLE_PERMISSIONS`
stayed exactly as they were (still an accurate reference table matching
`role_permissions`' seed data, still what `RolesPermissionsPanel`
displays) — only `TeamMember`/`TEAM_MEMBERS`/`ActivityEntry`/
`TEAM_ACTIVITY` and the `useTeamStore` mock itself were left behind by
the real Team page.

**Deliberately NOT touched: `hooks/use-current-member.ts` and
`stores/team.ts`.** Three other, still-mock features
(`custom-metrics`, `content-gallery`, `referral`) call
`useCurrentMember()` for their own "who am I, what can I manage"
role-gating, keyed against `stores/team.ts`'s mock member ids
(`"mem_owner"`, etc.). Rewiring that hook onto the real session's user id
would have silently broken every "is this mine" check those three
features run against their own still-mock seed data — they aren't wired
to a real backend yet, and giving them a real identity ahead of their own
backend-integration phase is exactly the kind of unrequested, silently
destructive change CLAUDE.md warns against. They keep working exactly as
before, on the old mock, until each gets its own Phase.

Invite creation/resend now show the real link in a copy-to-clipboard
dialog (same "reveal a secret once" pattern as
`features/settings/api-keys-panel.tsx`'s API key reveal) instead of a
"we sent an email" toast that would have been a lie — there's no email
delivery (see Phase 28A's own scope decision above).

## RBAC-aware navigation (a real gap found via manual testing)

Before this fix, a signed-in Viewer (whose only permissions are
`analytics.read`/`campaign.read`) could still click "Team" in the
sidebar — the link was unconditionally rendered — and land on a bare
`ErrorState` card reading "missing permission: team.read." The API was
doing exactly what it should; the frontend just wasn't hiding a link
that was always going to fail for that role, which is precisely what
§52's "frontend permissions are only UX" is supposed to prevent.

Fixed with `NavItem.requiredPermission` (`lib/nav.ts`) — set to
`"team.read"` on the Team entry only, the one item this phase's RBAC
actually covers — and a `visibleNavGroups(groups, permissions)` filter
applied in both `nav-content.tsx` (the sidebar) and `command-menu.tsx`
(⌘K), reading `useMe().data?.permissions`. `MemberList`/`member-columns.tsx`
additionally hide the "Invite member" button, disable the role `Select`,
and hide the row-actions menu entirely when the caller lacks
`team.write` (e.g. a Manager, who has `team.read` but not `team.write`)
— server-side 403s are still the real enforcement (verified via the
Viewer's own invite attempt correctly 403ing before this UI-gating
existed), this just stops offering controls that were never going to
work.

## Verified

`gofmt`/`go build ./...`/`go vet ./...`/`go test ./...` still green
(unchanged — no Go code changed this phase). Frontend: `next typegen`
(needed once for the new `/accept-invite` route's `PageProps` type),
`tsc --noEmit`, `eslint .`, `vitest run` (21 tests, unchanged), and
`next build` all clean.

Full manual browser pass (Claude-in-Chrome) against the real running
`apps/api` + `apps/web` dev servers on their default ports (`8080`/
`3000`, matching `APP_URL`/CORS exactly — this phase's own manual
Phase 28A pass had used a nonstandard `18080` for `apps/api`, which
would have failed CORS against a `3000`-origin browser):

- Visiting `/overview` signed-out correctly redirected to `/login`
  (proxy.ts). Signing up created a real org, redirected to `/overview`,
  and rendered the full authenticated app shell — sidebar showed the
  real org name (`WorkspaceSelector`), topbar showed the real user's
  initials/name/email (`UserMenu`).
- `/team` showed exactly one real member (the new Owner), "last active"
  genuinely populated. Invited a Viewer through the real "Invite member"
  sheet (role dropdown correctly excluded "Owner") — got back a real
  `http://localhost:3000/accept-invite?token=...` link in the reveal
  dialog.
- Opened that link in a second tab: the public preview correctly
  rendered "Join Browser Test Org, invited as Viewer" (localized to
  Russian per this browser's `Accept-Language`, confirming the new
  `auth` i18n namespace loads correctly). Submitting created the
  invitee's password and logged them straight into `/overview`.
- The invited Viewer's sidebar correctly had no "Team" link at all
  (the nav-gating fix above); their `GET /auth/me` showed exactly
  `["analytics.read","campaign.read"]`.
- Back as the Owner: suspended the Viewer via the real row-actions menu,
  changed their status, then removed them — member count went from 2
  back to 1 in the real table, no page reload. The Activity tab showed
  all four real audit-log entries (invited, invite accepted, suspended,
  removed) with human-readable labels and the correct actor names.
  Roles & Permissions tab still renders correctly (static, unchanged).
- Logged out via the real menu item — redirected to `/login`, and a
  subsequent direct navigation to `/team` redirected to `/login` again
  (cookie actually cleared, not just a client-side route change).
- Spot-checked that the three still-mock features this phase
  deliberately didn't touch (`/content-gallery`) still render correctly
  against their own mock store, confirming `useCurrentMember`/
  `stores/team.ts` were genuinely left alone.
- No console errors at any point in the flow.
- Cleanup: deleted every organization/user this manual pass created
  (`Browser Test Org` and its owner/invitee, plus `Curl Test Org` and
  its fixtures from Phase 28A's own manual pass, discovered still
  present in the dev database while re-checking row counts here).

**A second real bug caught during this cleanup, not code review:** the
`internal/auth` Go test suite's own `signupOrg`/`uniqueEmail` helpers
created real rows in the shared dev Postgres on every run but never
deleted them (unlike every other package's own `seedOrg` helper, which
does) — three earlier `go test ./internal/auth/...` runs during Phase
28A had left 51 organizations and dozens of orphaned `users` rows sitting
in the dev database. Fixed by adding `t.Cleanup` to both helpers
(`signupOrg` deletes the org, cascading its memberships/sessions;
`uniqueEmail` separately deletes the `users` row by email, since
`organizations` has no FK from `users` to cascade through) — verified by
running the suite again and confirming zero new rows survived. The
pre-existing 51-organization backlog, plus this phase's own manual-test
fixtures, were purged by hand afterward; the dev database now matches its
state before either phase's testing began (one pre-existing `Phase 27 Dev
Org` fixture, zero users, zero sessions).

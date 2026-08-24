# i18n hydration race — deterministic fix (server-side locale via cookie)

Closes the known issue `docs/postlanding.md`/`docs/pwa.md` documented and
`docs/frontend-i18n.md`'s "Server-side/SSR-perfect locale rendering"
bullet deferred: a `requestIdleCallback`-delayed `i18n.changeLanguage()`
call, fired from a post-mount effect, could still race a Suspense-
deferred hydration commit (any page whose list component calls
`useSearchParams()` for the Content Gallery `?gallery=<id>` integration —
Landings, PWA, Postlanding) and produce a real, reproducible "Hydration
failed" error. Confirmed still reproducible (~2 of 6 fresh navigations)
on both `/postlanding` and `/landings` during the Postlanding phase's
manual testing, despite three successive timing tweaks (fixed delay →
`React.startTransition` → `requestIdleCallback`) each having reduced,
never eliminated, the race.

## Why timing tweaks could never fully close this

The race wasn't really about *how long* to wait before calling
`changeLanguage()` — it was that the client had to render `DEFAULT_LOCALE`
first (matching the server, which never knew the visitor's real locale)
and switch afterward, and nothing client-side can know for certain that
every deferred Suspense hydration commit has finished before switching.
`requestIdleCallback` waiting for the main thread to go idle is a good
heuristic, not a guarantee.

## An `AskUserQuestion` confirmed the trade-off before implementing

The only way to fully eliminate the race — not just make it rarer — is
for the **server** to already know the correct locale before it renders
anything, so there is no "switch after mount" step left to race at all.
That requires reading a per-visitor value (a cookie) during server
rendering, which is a genuine Request-time API in Next.js's App Router
(`cookies()`/`headers()` from `next/headers`) — using either opts a
route out of static prerendering into per-request dynamic rendering.
Before implementing, this was confirmed explicit: **all 26 routes went
from `○ (Static)` to `ƒ (Dynamic)`** in `next build`'s output. Accepted
because every route already fetches its real data client-side via
TanStack Query — static prerendering was only ever serving an empty app
shell, so losing it costs little for this internal, authenticated
control-plane dashboard.

## The fix

`app/layout.tsx` (a Server Component, `async` now) reads
`cookies()`/`headers()`, resolves the locale via
`lib/i18n/locale.ts`'s `resolveLocale(cookieValue, acceptLanguageHeader)`
— persisted cookie choice wins; otherwise the `Accept-Language` header's
primary subtag, if supported; otherwise `DEFAULT_LOCALE` — and renders
`<html lang={locale}>` and `<I18nProvider initialLocale={locale}>`
directly. The client hydrates against that exact same value, passed as
a plain prop, never re-derived from `localStorage`/`navigator.language`.
No post-mount `changeLanguage()` call is needed to *match* what the
server sent, because the server and the client's first render now agree
by construction — there is nothing left to race.

### `lib/i18n/locale.ts` — split out to stay import-safe from a Server Component

`app/layout.tsx` has no `"use client"` directive, so it (and everything
it imports without going through a client boundary) is evaluated under
React's `react-server` build, which doesn't export `createContext` (a
client-only API) — and `react-i18next` creates its own context at module
load. Importing `lib/i18n/config.ts` (which unconditionally imports
`react-i18next`) directly from `app/layout.tsx` throws immediately:
`TypeError: (0, aa.createContext) is not a function`, surfaced as a
`next build` failure during "Collecting page data". `resolveLocale`,
`LOCALE_COOKIE`, `isSupportedLocale`, `SUPPORTED_LOCALES`,
`DEFAULT_LOCALE`, and the `Locale` type moved into a new
`lib/i18n/locale.ts` with zero `i18next`/`react-i18next` imports —
`app/layout.tsx` imports from there directly; `lib/i18n/config.ts`
re-exports the same names so every existing `"use client"` call site
(`hooks/use-locale.ts`, `components/language-switcher.tsx`, this
module's own tests) keeps importing from `config.ts` unchanged.

### `createI18nInstance` — a fresh instance per call, never a shared singleton

The previous design had exactly one process-wide `i18next` instance,
mutated in place via `changeLanguage()`. That was never safe for
per-request server locale resolution: `I18nProvider`'s render function
now runs once **server-side per request** (Next.js can have multiple
requests — different visitors, different resolved locales — in flight
concurrently on the same Node process) in addition to once **client-side
per browser tab**. A shared mutable global's `.language` would race
across those concurrent requests. `lib/i18n/config.ts`'s
`createI18nInstance(locale)` calls `i18next.createInstance()` and returns
a brand-new, independently-configured instance every time; `I18nProvider`
creates its own via `React.useState(() => createI18nInstance(initialLocale))`
— the lazy-initializer form runs exactly once per component lifetime
(once for this server render, independently once for the client's
hydration render), so every render path gets its own instance with
nothing shared to race. Verified directly:
`TestReplayNetworkLookup`-style isolation test in `config.test.ts`
("two instances are fully independent — changing one never affects the
other").

### Persisting a choice: a plain client-writable cookie, no Server Action needed

`I18nProvider`'s `languageChanged` listener (fired by the switcher's
`i18n.changeLanguage()` call, a live in-session update — not part of
initial hydration, so it can never race anything) now writes
`document.cookie` directly instead of `localStorage`. Cookies are sent
to the server automatically regardless of how they were set — a plain,
non-`httpOnly` `document.cookie` write is exactly what the next
`cookies()` read in `app/layout.tsx` needs, no Route Handler or Server
Action round-trip required. `document.documentElement.lang` is still
kept in sync in the same listener, but only matters for a *live* switch
now — the initial value is already correct from the server-rendered
HTML.

## Verified

- `go build/vet/gofmt/test ./...` — unaffected (no backend changes this
  phase).
- `tsc --noEmit`/`eslint`/`vitest run` (21 tests, up from 15 — new
  `resolveLocale` precedence tests and `createI18nInstance` isolation
  tests) all clean.
- `next build` (production): compiles clean, confirmed every route is
  now `ƒ (Dynamic)` (was `○ (Static)`), matching the accepted trade-off.
- Confirmed via plain `curl` (no browser, so no possibility of
  client-side JS masking a server-rendering bug) that the raw HTML
  itself is correct before any JS runs: `curl -H "Cookie: flox-locale=ru"`
  and `curl -H "Accept-Language: ru-RU,ru;q=0.9"` (no cookie) both
  returned `<html lang="ru">` with Russian text (`Постлендинг`) already
  in the initial response body; a request with neither returned
  `<html lang="en">` with English text.
- Full manual browser pass against `next start` (the actual production
  server, not `next dev`) + real `api`: set the `flox-locale=ru` cookie,
  then performed 10 full fresh navigations (real browser navigations,
  not client-side transitions) across `/postlanding`, `/landings`,
  `/pwa`, including 4 with the exact `?gallery=<id>` query param that
  drives the Suspense boundary the original race depended on. Zero
  hydration errors across all 10 — read directly from the browser
  console each time, not inferred from a screenshot. Confirmed the live
  language switcher still works (instant switch, no reload, cookie
  correctly updated to `en`), and that a subsequent fresh navigation
  correctly rendered the newly-persisted `en` choice server-side with,
  again, zero hydration errors.

## `docs/frontend-i18n.md` updated

Its "What's deliberately deferred" bullet on SSR-perfect locale
rendering is removed (done, not deferred); "Where translations live",
"Testing", and "Accessibility" sections updated to match the
`locale.ts` split and the new server-resolved-`<html lang>` behavior.

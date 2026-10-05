# PWA stage: serving install pages (§25, §28, §43, §73)

A flow's PWA stage (`flows.pwa_enabled` + `pwa_id`, type `internal`) now
reaches real visitors: `apps/tracker` serves an installable PWA page,
records the PWA funnel events, and continues to the destination when the
installed app is launched. Code: `apps/tracker/pwa.go`; tests:
`apps/tracker/pwa_test.go`.

## Funnel

```
click /t/{slug} ──► [internal landing] ──► /pwa/{token}/ ──► install ──► app launch ──► destination
   SOURCE_CLICK      LAND_VIEW/LAND_CLICK    PWA_VIEW        PWA_INSTALL   PWA_OPEN
```

- No landing: `/t/` 302s straight to `/pwa/{token}/`.
- Internal landing first: its `trackUrl` click-through (`/lc/`) records
  LAND_CLICK, then 302s to `/pwa/{token}/` instead of the destination.
- External landing first: we can't link onward from an advertiser's page,
  so the PWA behind it is never reached — and never attributed.
- Skipped (302 to the destination as before): PWA type `external`/
  `ios_app` (neither carries a target URL in this schema — nothing to
  serve, nothing guessed), a paused PWA, or no `LANDING_LINK_KEY`.

## Routes (all on one signed token)

Everything under `/pwa/{token}/` shares the click's signed link
(`apps/internal/landinglink`, now with `PwaID`), so the page, its
manifest, the service worker's scope and the installed app's `start_url`
stay bound to the original click_id for as long as the app is installed.

| Route | Does |
|---|---|
| `GET /pwa/{t}/` | install page → PWA_VIEW (or the WebView bounce page, below) |
| `GET /pwa/{t}/manifest.webmanifest` | web app manifest: name, short_name, description, colors, icon (192 + 512 entries), `display: standalone`, `scope = /pwa/{t}/`, `start_url = /pwa/{t}/open` |
| `GET /pwa/{t}/sw.js` | pass-through service worker (fetch handler for installability, no caching) |
| `POST /pwa/{t}/installed` | the page's beacon on `appinstalled` / accepted prompt → PWA_INSTALL |
| `GET /pwa/{t}/open` | installed app's start_url → PWA_OPEN → 302 destination |
| `GET /pwa/{t}/continue` | "continue without installing" → 302 destination, no event |

The manifest and `open` accept tokens older than the 30-day link max age
(`Signer.VerifyAnyAge` — signature still required): an installed app has
to keep working. A PWA paused or deleted after the link was issued sends
the page's visitor straight on to the destination.

Events from these routes carry the token's routing/attribution context
(campaign, flow, offer, landing, pwa, utm, subs) plus geo/device
re-derived from the request. `pwa_id` is on every event of a journey that
goes through the stage — ClickHouse `chstore/schema/010_pwa_id.sql`, the
`pwa` report dimension (`docs/analytics-report-api.md`).

## The page

Rendered server-side from `apps/tracker/templates/pwa.html` (a plain
`html/template` file, compiled into the binary with `go:embed` — edit the
file, rebuild the tracker; not Twig, which is PHP) (`html/template`, everything escaped; colors only
admitted as `#rgb`/`#rrggbb`) and laid out like the editor's preview card
(`features/pwa/pwa-preview-card.tsx`): icon + name + developer header,
install button, screenshot strip (fixed height, natural widths), "About"
(5-line clamp, → opens a details sheet), "Updated on", "What's new". The
accent is the manifest `theme_color`; light/dark follows `background_color`
by the same BT.601 brightness check as the card. Russian or English copy by
`Accept-Language`.

Install flow mirrors the card: the button appears on `beforeinstallprompt`;
on an accepted prompt it collapses, the icon becomes a circle with a ring
filling around it and the developer line reads "Please wait…" (1.8 s), then
"Open" (→ `open`). iOS gets "Share → Add to Home Screen" steps (Safari has
no install event — iOS installs show up as PWA_OPEN when launched; no
IOS_INSTALL is fabricated); an already-installed app opened in the browser
goes straight to `open`; every browser gets "continue without installing"
so no visitor is stranded.

Fields shown (all plain facts from the PWA): name, icon, developer,
description, screenshots, what's new, version, category, interactive
elements, ads / in-app-purchase notes, updated date. The note line under the
developer appears only when ads/purchases are set — the card's "no ads or
paid content" fallback is an assertion, so it is not carried over.

**One template.** Every render — a visitor from a campaign and the
constructor's signed test link alike — uses `templates/pwa.html`, the editor
card one to one: "Apps" header, rating and star distribution, review count,
downloads, size, Share/Wishlist, screenshots, About + details sheet, data
safety, ratings and reviews, What's new, footer, bottom nav. All numbers are
exactly what the operator typed in the constructor form; the test link only
adds a "test mode" banner and records no events. (Decision 2026-10-05, the
operator's: visitors see the full card. Earlier versions withheld the numbers
from visitors.) A template execution error redirects the visitor to the
destination rather than serving half a page.

**Theme.** The PWA's "Тема" setting is light, dark or auto. Light/dark are
fixed and derived from `background_color`. Auto (`pwas.theme_auto`, migration
00037) renders the light palette as the base and swaps in the dark one under
`@media (prefers-color-scheme: dark)`, with `color-scheme: light dark`; the
constructor preview follows the browser's own preference the same way.

## In-app WebView bounce (§28/§73)

With the PWA's `bounceInAppWebview` on, a request whose user agent
self-identifies as an in-app browser (`FBAN/`, `FBAV/`, `FB_IAB`,
`Instagram`, `BytedanceWebview`, `musical_ly`, `TikTok`, `Line/`,
`Snapchat`) gets a bounce page instead: Android → an `intent://` URL
handing the same page to the default browser (with a fallback URL); iOS →
`x-safari-https://`. No PWA_VIEW on the bounce — the real view is recorded
when the page loads in the browser. This is the generic "can this browser
install a PWA" capability §73 requires, never visitor/moderator detection
(which §73 forbids). Telegram's in-app browser has no reliable UA marker
and is not bounced.

## Testing from the constructor

The PWA editor's header has **Test page** (`Тестовая страница`):
`POST /pwas/{id}/test-link` (`apps/internal/pwa/testlink.go`) mints a
signed tracker link marked `Test`, for the *saved* PWA. The tracker serves
it exactly like a visitor's (manifest, service worker, install prompt,
bounce), shows a "test mode" banner, and records **no events**; continue/
open lead back to the editor. Works even for a paused PWA. Needs
`LANDING_LINK_KEY` and `TRACKER_PUBLIC_URL` on apps/api (dev: compose and
`.env.example`); otherwise the button answers 503. `localhost` counts as a
secure context, so install works in dev without TLS.

## Not done

- Postlanding stage serving (and its report dimension) — next.
- The landing's `asPwa` flag; PWA types `external`/`ios_app` (no URL
  fields exist for them).
- Push notifications (`NOTIFICATION_*`) from the installed app.

## Verified

- `go test ./...` with the full env (PG + CH + Redis + MinIO, e2e). New:
  `tracker/pwa_test.go` — direct funnel (page/manifest/sw/installed/open/
  continue, exactly SOURCE_CLICK+PWA_VIEW+PWA_INSTALL+PWA_OPEN on one
  click_id with pwa_id; page contains no "Google Play" and none of the
  listing stats even when they're set), landing→PWA, skipped stages,
  test link records nothing, WebView bounce; `landinglink` VerifyAnyAge;
  `pwa/testlink_test.go` (503 unconfigured, claims, cross-tenant);
  chstore/attribution/conversion pwa_id. Benchmarks unchanged
  (`docs/performance.md`).
- Live in Chrome: the editor's Test page opened the install page on the
  tracker — service worker active, manifest served, Chrome fired
  `beforeinstallprompt` (install button shown, i.e. installable), test
  banner, no store branding/stats; "continue" returned to the editor; zero
  events recorded. Real traffic through a campaign with the PWA stage:
  ClickHouse got SOURCE_CLICK×5, PWA_VIEW×4, PWA_INSTALL×2, PWA_OPEN×2,
  all with pwa_id (the 5th click was an in-app-WebView request that got
  the `intent://` bounce and correctly no view); the report filtered by
  PWA showed the name and 5 clicks. The native install dialog itself was
  not clicked (browser-automation limitation). Fixtures removed.

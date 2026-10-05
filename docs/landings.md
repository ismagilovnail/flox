# Landings (§28)

Two kinds of landing, fixed at creation:

- **external** — a URL the advertiser already hosts. Name, URL, optional
  title/logo, status.
- **internal** — an uploaded **ZIP bundle**, modelled on the reference
  product's "custom landings" (adset.pro, *Пользовательские лендинги*):
  logo, name, title, archive, status; a template editor with live preview;
  re-upload that keeps the landing's ID; a platform runtime injected into
  every page.

Package: `apps/internal/landing` (handler → service → repository, plus
`bundle.go` and `render.go`). Migration `00036_landing_bundles.sql`.

## ZIP bundles

`POST /landings/upload` (multipart: `name`, `title`, `logoUrl`, `status`,
`archive`) creates an internal landing. `POST /landings/{id}/archive`
(multipart: `archive`) re-uploads one in place. Both return
`{landing, warnings}`.

Archive contract (`bundle.go`, enforced server-side; the form only
pre-checks extension/size to fail fast):

- `index.html` or `index.twig` at the archive root — or at the root of its
  single top-level folder, which is stripped (the common "compressed the
  folder, not its contents" archive). Both present → `index.html` wins.
- Paths are cleaned and anything that could escape the bundle is rejected
  (`..`, absolute paths, drive letters — zip-slip). `__MACOSX/`,
  `.DS_Store`, `Thumbs.db` are dropped.
- Limits: 50 MB archive, 100 MB declared uncompressed total (archive/zip
  itself errors if a file's real size differs from its declared one, so the
  declared sum is a sound zip-bomb guard), 1000 files, 2 MB UTF-8 entry
  file.
- Root-absolute references in the entry file (`/style.css`,
  `url('/bg.jpg')`) are returned as `warnings`, not errors — they upload
  fine but won't resolve against the bundle, same as the reference
  product. The UI shows them in a toast.

Storage layout:

- The **entry file lives in Postgres** (`content`, `entry_file`) — it's
  what the template editor edits and what gets rendered. It is never put in
  object storage, so no un-instrumented public copy of the page exists.
- **Every other file** goes to object storage under
  `landings/{org}/{id}/{version}/…`, `version` a fresh ULID per upload
  (`bundle_prefix` column). The public base URL of that prefix is the
  landing's `url` — derived at read time (`Service.decorate`), never
  stored, so a changed `S3_PUBLIC_ENDPOINT` can't leave stale values.
- Re-upload writes the new version under its own prefix, switches the row
  in one `UPDATE` (`Repository.SetBundle`), then removes the old version —
  a failed upload never leaves a half-replaced landing. The archive's entry
  file replaces any template edits (the archive is the new source of
  truth; the form says so).
- Duplicate copies the bundle server-side (`storage.CopyPrefix`) to the
  copy's own prefix, so re-uploading either one never touches the other.
- Delete removes the row, then every version under `landings/{org}/{id}/`
  (best-effort, logged on failure — an orphaned object costs storage, not
  correctness).
- No object storage configured → external landings and all reads keep
  working; upload/re-upload/duplicate-of-a-bundle answer 503, same stance
  as Content Gallery.

`type` can't be changed after creation (a PATCH resending the same type is
accepted). Switching would mean conjuring or discarding a whole archive
through a metadata PATCH. Pre-existing internal landings (HTML typed into
a textarea before this change) keep working: their `content` is their
entry file, they just have no assets until re-uploaded. Their old
`cdn.floxlink.io/lnd/<slug>` URL — which nothing ever served — was cleared
by the migration.

Title and logo are required for internal landings, optional for external
ones. The logo is picked from Content Gallery through the same 1:1 crop
picker the PWA icon uses.

## Injected runtime (`render.go`)

`Render(source, assetBaseURL, trackURL)` is a pure function, shared by the
editor preview and the tracker serving real visitors — what
the operator previews is exactly what visitors get. It:

1. Substitutes `{{ trackUrl }}` (whitespace-tolerant), HTML-escaped as
   Twig's autoescape would. Only this macro is supported — there's no Twig
   engine in Go, and full Twig is out of scope.
2. Injects at the very top of `<head>` (synthesizing one if needed, keeping
   a doctype first so the page doesn't drop into quirks mode):
   - `<base href="{assetBaseURL}">` so relative asset paths resolve to the
     bundle — skipped if the page sets its own `<base>`;
   - the runtime script: a global `trackUrl` (JSON-encoded, so it can't
     break out of the `<script>`), a global `engine_onClickButton()` that
     navigates to `trackUrl`, every `<a>`'s `href` set to `trackUrl` on
     DOMContentLoaded, and one capture-phase delegated click listener that
     routes any `<a>` click — including anchors a framework renders later —
     through `engine_onClickButton`. `data-flox-skip` on an anchor opts it
     out (e.g. a privacy-policy link); the reference product has no such
     escape hatch.

The reference product's push-subscription and PWA-install variants of
`engine_onClickButton` aren't implemented — those belong to the PWA domain.

## Template editor

`POST /landings/{id}/preview` (`{content?}`) returns `{"html": …}` —
`content` rendered via `Render` with `trackUrl =
"javascript:void(0)"` (not `"#"`: with the injected `<base>`, `"#"`
resolves to the bundle URL and a click would navigate the preview away).
JSON, never `text/html`: user HTML must not run on the API's own origin.
Gated by `content.write` like every other POST.

Frontend (`landing-template-editor.tsx`): full-screen dialog, CodeMirror 6
on the left (`html-code-editor.tsx`, loaded via `next/dynamic` so it stays
out of every other page's bundle; one-dark theme in dark mode), live
preview on the right, debounced 400 ms, stale responses dropped. The
preview iframe is `sandbox="allow-scripts"` **without**
`allow-same-origin`: the operator's scripts run, on an opaque origin with
no access to the app's cookies or storage. Save = `PATCH {content}`;
closing with unsaved edits asks first. Reachable from the edit page and
from a row action in the list.

## Serving visitors (`apps/tracker`)

When routing picks a flow whose landing stage is enabled
(`flows.landing_enabled` + `landing_id`, loaded by `routingstore` in the
same query as the flows, org-scoped on the landing side too):

| Landing | Tracker does |
|---|---|
| internal, active or archived | serves the page **as the response to `/t/{slug}` itself** (200, no extra redirect hop): `landing.Render(content, assetBaseURL, trackURL)`, `Cache-Control: no-store`. Records `SOURCE_CLICK` + `LAND_VIEW` on one click_id. |
| internal, paused | skipped — 302 to the flow's destination, as if no stage. |
| external (not paused) | 302 to the landing's URL instead of the destination. |
| none / stage disabled | 302 to the destination, unchanged. |

Archived landings are still served: archiving only hides a landing from
the flow picker (the archive confirmation already promised existing flows
keep working). The page source is read by a separate primary-key query
(`LoadLandingPage`) only when an internal landing is actually served —
never as part of loading routing config, so a 2 MB page never rides along
on a plain redirect. If it fails to load, the click is redirected to the
destination and the error logged: a broken landing never takes the
redirect down.

`trackUrl` is an absolute signed link on the same tracking host:
`{scheme}://{host}/lc/{token}` (absolute because the page carries a
`<base href>` to object storage). `apps/internal/landinglink`: HMAC-SHA256
over a compact JSON payload — org, campaign, click_id, stream set, flow,
landing, destination, config version, issue time, and the click's utm/
sub/external-click-id/referrer — 30-day max age. Signed because the
destination is in it: unsigned, `/lc/` would be an open redirect on every
customer's tracking domain and a way to inject LAND_CLICKs into any org.
Why a token and not a lookup: click rows reach ClickHouse asynchronously
(may not exist yet a second later), and Redis is cache-only (CLAUDE.md
#4).

`GET /lc/{token}`: verify (any failure → 404, nothing recorded), classify
the request for geo/device (same visitor), record `LAND_CLICK` with the
token's routing + attribution context, 302 to the destination the click
was routed to when the page was served. No database access. Every click
on a landing records a LAND_CLICK (a visitor clicking twice is two).

Config: `LANDING_LINK_KEY` (32+ bytes base64; dev value in
`.env.example`/compose). Unset → tracker warns at startup and skips
internal landing stages (straight to destination, as before). The tracker
also reads `S3_ENDPOINT`/`S3_PUBLIC_ENDPOINT`/`S3_BUCKET` to build asset
URLs — no storage credentials, it never touches objects.

Still not done: the landing's `asPwa` flag, the PWA/postlanding stages,
and the push/PWA variants of `engine_onClickButton`. (The `landing`
report dimension followed in its own change — every event the tracker
emits for a shown landing carries `landing_id`; see
`docs/analytics-report-api.md`.)
The flow editor still only offers a "preview" link for external landings
(an internal one's `url` is an asset base, not a page).

## Verified

- `go build/vet/gofmt/test ./...` green. `bundle_test.go`/`render_test.go`
  are pure unit tests (layouts, zip-slip, limits, content types, warnings;
  injection order, doctype, macro escaping, `<base>` precedence, script
  breakout). `landing_test.go` runs against real Postgres **and** MinIO:
  full archive lifecycle (upload, assets publicly served with correct
  content types, template edit, preview, re-upload keeps the ID and 404s
  the old version, duplicate gets its own copy, delete cleans up only its
  own objects), bad archives, type immutability, storage-unavailable 503,
  and cross-tenant isolation over every endpoint including archive,
  preview and duplicate. apps/api's RBAC coverage test caught the preview
  route ungated on the first run — fixed.
- Real browser pass (ru) against the running stack: empty-form inline
  errors; created a ZIP landing (logo uploaded + cropped through the
  gallery picker, archive with a top-level folder, a `__MACOSX` entry and a
  root-absolute `/abs.css`) — warning toast named exactly `/abs.css`; edit
  page showed entry file/count/size/asset URL with type locked; template
  editor rendered CSS + SVG from the bundle, live-updated on typing,
  clicking a link in the preview stayed put, close-with-unsaved-edits
  confirmed, save toasted; re-uploaded an `index.twig` archive — same ID,
  new version, `{{ trackUrl }}` substituted in preview; list showed logo,
  title, "index.twig · 2 файла · 239 B"; duplicated; deleted both through
  the API and confirmed their assets 404. No console errors. All
  fixtures (landings, gallery logos, a throwaway signup org) removed.

Phase B (serving):

- `go build/vet/gofmt/test ./...` green. `landinglink_test.go`: round
  trip, tampered payload, other key, garbage signature, expired, future-
  dated, missing destination, key parsing. `tracker/landing_test.go`
  against real Postgres: page served with `<base>`, absolute signed
  trackUrl and substituted macro, then click-through 302s to the offer
  with SOURCE_CLICK/LAND_VIEW/LAND_CLICK on one click_id carrying utm/
  subs; paused skipped, archived served, no key → skipped, external →
  its URL; forged/garbage tokens 404 with no events; `LoadLandingPage`
  org-scoped. Benchmarks: `docs/performance.md`.
- Live, against the running dev stack: created a campaign + flow with an
  uploaded landing through the real API, opened the tracking link in
  Chrome as a visitor — page rendered with CSS and image from MinIO, links
  bound, `data-flox-skip` left alone, the page's own script read
  `trackUrl`; clicking a non-link button bound with
  `engine_onClickButton` landed on the offer. The rows arrived in
  ClickHouse through the real worker (SOURCE_CLICK, LAND_VIEW, LAND_CLICK,
  same click_id, utm/sub intact). All fixtures removed (Postgres,
  ClickHouse, object storage).

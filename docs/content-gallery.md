# Content Gallery (§30.8)

Two sections on `/content-gallery`: the **team library** (images and videos
the team uploads; tenant-scoped, object storage — `internal/contentgallery`) and the
**system catalog** (content shipped with FLOX, the same for every
organization). §30.8: "distinguish system-provided gallery content from
team-uploaded content".

## Video (2026-10-15)

MP4 and WebM up to 20 MB (the reference product's video cap), **stored as
uploaded**. The type is checked from the file's own bytes
(`http.DetectContentType`: an MP4 `ftyp` box with an mp4 brand, WebM's
EBML header) against its extension. Other formats (MOV first of all —
iPhone QuickTime files are not MP4s) are converted: see "Transcoding"
below.

The uploading browser reads the video (`lib/video-poster.ts`): a frame
~0.5 s in, scaled to 640 px on the long side, as a JPEG **poster**; the
**duration** and **frame size**. They go with the upload (`poster`,
`durationMs`, `width`, `height` form fields; migration 00082 adds
`poster_object_key`, `duration_ms`, `width`, `height`) — display only,
range-checked; a poster must be a JPEG/WebP/PNG up to 2 MB. A video the
browser cannot decode (or that takes over 8 s) uploads without them.
Deleting a video deletes its poster object too.

UI: the gallery accepts videos (the upload cell says so); cards show the
poster (or a film icon), the duration and a play button that opens a
player with the file's facts and its link to copy; a type filter (all /
images / videos). Every picker that attaches a gallery file to a field
(PWA icon and screenshots, landing logo, comment avatar) offers images
only — it lists `type=image` and its upload cell accepts images only.

Tests: `TestUploadVideo` (MP4 and WebM as sniffed, poster and metadata
stored and listed, the type filter, bad metadata and posters refused, the
poster object removed on delete), fake-MP4 refusals —
mutation-checked; `lib/video-poster.test.ts`. Live: a 2 s WebM recorded in
Chrome uploaded through the gallery's own file input → poster, 1946 ms,
320×568; played in the dialog; archived and deleted (video and poster gone
from storage).

## Transcoding (2026-10-24, GAP-1)

The owner changed the earlier "no ffmpeg on the server" choice: MOV, M4V,
3GP, AVI and MKV are accepted and turned into MP4 by `apps/worker`.

- **Upload** (`Service.uploadSource`): up to **200 MB**, checked from the
  first bytes (`sniffSource`: an ISO-BMFF `ftyp`/`moov`/`mdat`/`wide`/
  `free`/`skip` box for MOV/M4V/3GP, `RIFF…AVI ` for AVI, the EBML header
  for MKV). Stored as `{org}/{id}-source{ext}` with `processing_status =
  'processing'` (migration 00100) and no URL — browsers may not play the
  source. The browser's poster and metadata are not read or taken for it.
  The API's 5-minute read timeout puts a 200 MB upload at ≥ ~5.5 Mbit/s.
- **Worker** (`contentgallery.Processor`, every 5 s when idle, one video
  at a time): claims the oldest waiting file with a 20-minute lease
  (`processing_started_at`, `FOR UPDATE SKIP LOCKED` — a worker that died
  gives it back), downloads it, `ffprobe`s it, and refuses a file with no
  video stream or longer than 10 minutes. `ffmpeg`: H.264 (veryfast,
  CRF 23, yuv420p) + AAC 128k stereo, long side at most 1920 (rotation
  applied, so the result is upright), metadata stripped, `+faststart`,
  2 threads. The MP4 must fit the gallery's 20 MB; a JPEG poster at
  0.5 s, 640 px long side. Then the row gets the MP4's key, byte size,
  duration and frame size, `ready`, and the source is deleted.
- **Failures:** what the tools refuse (unreadable, no video, too long,
  MP4 over 20 MB, over 15 minutes of work) is `failed` with
  `processing_error`, shown on the card; not retried. Storage/database
  errors are retried when the lease runs out; a file taken 3 times
  without an outcome (it keeps killing the worker) is `failed`. A failed
  file keeps its source until it is deleted.
- **Deleted mid-way:** the update only applies while the row still points
  at the source, so the MP4 and poster just made are removed instead.
- **Config:** `FFMPEG_PATH`/`FFPROBE_PATH` (`docs/environment.md`); the
  worker image installs `ffmpeg`. Without it or without object storage the
  worker logs a warning and files stay processing.
- **UI:** the upload cell accepts the formats and says they are
  converted; a toast after the upload; the card shows "Converting…" or
  "Couldn't convert" with the reason, without a play button; the list
  refreshes every 5 s while a file on it is processing. Pickers list
  images only, so a processing video cannot be attached anywhere.

Tests: `TestTranscodeReplacesSourceWithMP4` (processing without a URL,
another organization's processor takes nothing, then MP4 + poster stored
and the source gone), `TestTranscodeFailures` (no video, too long, given
up after 3 attempts; delete removes the source), `TestTranscodeOfADeletedFileLeavesNothing`
(mutation-checked), `TestSniffSource`,
`TestFFmpegTranscodesAMOV` (real ffmpeg: a rotated 2560×1440 MOV →
upright 1080×1920 H.264/AAC MP4 and a 360×640 poster; junk → failed; skips
without ffmpeg — run it in the worker container), the oversize/fake MOV
refusals; `gallery-file-card.test.tsx` (mutation-checked),
`lib/video-poster.test.ts`. `Processor.OrgID` scopes a test's processor to
its organization so it leaves the shared dev database's rows alone.

## System catalog

`apps/internal/contentgallery/system/` is embedded in the API binary:
`catalog.json` lists each item (en/ru name and description, tags, files),
next to its images and landing bundles (`system/README.md`). The catalog is
parsed and checked at start-up — unknown fields or kinds, missing files,
missing translations or duplicate ids stop the API (and fail
`TestSystemCatalog`), so a broken catalog never reaches a request.

Kinds and their hand-off ("use", `POST /content-gallery/system/{itemId}/use`,
`content.write`):

| Kind | Use | Result |
|---|---|---|
| `creative` | the image is copied into the team library | `{kind, file}` |
| `landing` | the template's logo is copied into the library, its folder is zipped and created as an internal landing (`landing.Service.CreateFromArchive`, same checks as an upload); optional `{name}` | `{kind, landing}` — the web app opens the landing's page |
| `pwa` | icon and screenshots are copied into the library; nothing else is saved | `{kind, pwa}` — the PWA form's fields with the copied URLs; the web app opens `/pwa/new` filled in (`stores/pwa-template-draft.ts`) |

Copies go through `UploadFile`, so they get the same type checks, size cap,
storage layout and public URL as a team upload, and from then on are plain
team content (private to the organization). A landing whose creation fails
after its logo was copied leaves the logo in the library.

Reading: `GET /content-gallery/system` (the catalog; `previewPath` and, for
landings, `assetBasePath` are relative to the API origin) and
`GET /content-gallery/system/{itemId}/preview?assetBase=…` — a landing
template rendered by `landing.Render` with links disabled, as JSON. The web
app shows it in an iframe with `sandbox="allow-scripts"` (no same-origin),
like the landing template editor.

Assets: `GET /v1/gallery-system/assets/{path}` is public and cacheable
(no tenant data) and serves **only `.png` and `.css`** from `system/`, with
`nosniff`. HTML is never served from the API's origin.

All system content is generic sample material with invented names; the PNGs
are generated by `contentgallery/gen/make_assets.py`.

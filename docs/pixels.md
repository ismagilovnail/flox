# Pixels (§29)

A pixel reports FLOX events to an ad platform so the platform can optimize
delivery. **Sending** (below) is how events reach the platform; the
sections after it describe the CRUD phase that came first.

## Sending (server-side, §29 / §74 `PixelProvider`)

Until 2026-10-11 pixels were saved and attached to stream sets but nothing
was ever sent (`docs/spec-conformance-2026-10-11.md`, gap 1). Now:

### Which events, which pixels

A pixel sends the events it is set to (`events`) for clicks that went
through a stream set it is attached to (`stream_set_pixels`), while it is
active. Reportable events (`pixel.ValidEventTypes`): `PWA_INSTALL`,
`TG_START`, `TG_JOIN` (since 2026-10-15), `CPA_HOLD`, `CPA_ACCEPT`,
`CPA_REDEP` — what FLOX produces and a platform optimizes for.
`NOTIFICATION_SUBSCRIBE` is produced (push) but not offered yet: it needs
its platform mapping.

| Event | Queued by | When |
|---|---|---|
| `CPA_*` | `conversion.PostgresStore.Record`, in the conversion's own transaction (`conversion.PixelWriter`) | the conversion is recorded as a success and attributed to a click with a stream set |
| `CPA_*`, attributed late | `reattribute`, in the transaction that writes the attribution | an `unknown_click` conversion whose click turns up: its pixels had no stream set to go on before, so — unlike the outgoing postback, which already left — they get it now |
| `PWA_INSTALL` | `pixelsend.TriggerSink`, wrapping the worker's ClickHouse writer (event queue flusher and NATS consumer) | the event is taken off the stream, before it is written |
| `TG_START`, `TG_JOIN` | the same `TriggerSink` | the bot's webhook event (`docs/telegram.md`) is taken off the stream; it carries the click's stream set |

Unattributed conversions reach no pixel: pixels belong to a stream set,
and only the click knows which one.

### Queue: `pixel_deliveries` (migration 00069)

One row per (pixel, source event), `UNIQUE (pixel_id, source_event_id)`:
a retried batch or transaction never queues twice. `source_event_id` is
the conversion's `postbacks` row id or the tracker event's id, and is also
sent as the platform's `event_id`, which the platforms deduplicate on — so
a resend after a crash is not counted twice either.

`payload` is a `pixel.Event` captured when the event happened: type, time,
click id and click time, revenue/currency and the USD value at the event
date's rate (§50-FX), the visitor's IP, user agent, `fbclid`/`ttclid`, and
the macro values for generic URLs. For conversions these come from the
attributed click — `click_lookup` gained `ip`, `user_agent`,
`fb_click_id`, `tt_click_id` (ClickHouse `schema/015`; clicks from before
it have them empty). For `PWA_INSTALL` the IP/UA are the install request's
and the platform click ids come from the click (`ClickResolver`). Telegram
events arrive from Telegram's servers, not the visitor, so they carry no
IP/UA: the click's are sent (any event without both takes the click's).

Same mechanics as `postback_deliveries`: fair claim across organizations
(`internal/pgqueue`), 10 sends at a time, 10 s per send, the reaper returns
abandoned claims, `queuestats` reports depth and age. Finished rows are
deleted after `RETENTION_PIXEL_DELIVERIES_DAYS` (default 30) — they hold IPs
and user agents.

### Outcomes

- 2xx (TikTok: body `code` 0) → `success`.
- No response, timeout, 5xx, 429/408, Facebook throttling codes, TikTok
  40100/5xxxx → `retrying` on the postback schedule
  (`postback.NextAttemptDelay`, 8 attempts over ~21 h), then `dead`.
- Any other refusal (bad token, unknown pixel, 4xx) → `dead` at once:
  repeating the same request cannot succeed. So is a pixel that cannot
  send as configured (no pixel id, no token, no URL) or was paused after
  the event was queued. The message says which.
- Deleted pixel: its rows go with it (`ON DELETE CASCADE`).

Unlike outgoing postbacks, which retry every failure, a platform's 4xx is
final: these APIs answer 4xx for configuration errors (token, pixel id),
and retrying them for 21 hours only hides the problem from the delivery log.

### Providers (`internal/pixelsend`, one `Sender` each)

| Provider | API | Auth | Matching data sent |
|---|---|---|---|
| facebook | Meta Conversions API `POST graph.facebook.com/v21.0/{pixel}/events`, `action_source=website` | `access_token` query parameter | `client_ip_address`, `client_user_agent`, `fbc` = `fb.1.{click ms}.{fbclid}`, `external_id` = SHA-256(click id) |
| tiktok | Events API 2.0 `POST business-api.tiktok.com/open_api/v1.3/event/track/`, `event_source=web` | `Access-Token` header | `ttclid`, `ip`, `user_agent`, `external_id` = SHA-256(click id) |
| snapchat | Conversions API v3 `POST tr.snapchat.com/v3/{pixel}/events`, `action_source=WEB` | `access_token` query parameter | `client_ip_address`, `client_user_agent`, `external_id` = SHA-256(click id) |
| generic | `GET` of the pixel's URL template, macros resolved (`internal/macro`); `{status}` is the event name | — | whatever the template asks for |
| twitter | none: cannot be created or switched to; existing ones are kept, editable, and send nothing | X's Conversions API needs OAuth 1.0a app + user credentials (4 secrets) FLOX does not have — the owner's decision 2026-10-11 | — |

- **Value**: USD at the event date's rate when known, else the reported
  amount in its own currency, else none — never a made-up 0. Facebook
  rejects a `Purchase` without a value; that shows up in the log.
- **Event names**: per provider defaults (`pixel.DefaultEventNames`: FB
  Lead/Purchase/CompleteRegistration, TikTok SubmitForm/CompletePayment/
  CompleteRegistration, Snap SIGN_UP/PURCHASE; TG_START → FB/TikTok
  `Contact`, Snap `CUSTOM_EVENT_1` (Snap has no standard "contact" event);
  TG_JOIN → `Subscribe` / `SUBSCRIBE`), overridable per event
  (`event_names`). `GET /v1/pixels/event-names` serves the defaults to the
  form.
- **Test mode**: `test_event_code` (Facebook, TikTok) sends to the
  platform's test view.
- The tokens are in the query string for Facebook and Snapchat (their
  documented way): transport errors are reported without the URL
  (`*url.Error` unwrapped), and the event itself is never logged (§54).
- `event_source_url` is not sent: FLOX does not keep the landing URL of a
  click. Meta lists it as recommended for website events.
- All calls go through the worker's `SafeDialContext` client (SSRF guard,
  §54) — a generic pixel's URL is operator-supplied.

### Credentials

`access_token` is AES-256-GCM ciphertext under `PIXEL_ENCRYPTION_KEY`
(api writes, worker reads; both refuse to start without it). The API never
returns it: `hasAccessToken` + `accessTokenLast4`. PATCH without
`accessToken` keeps it, `""` removes it. Duplicating a pixel copies the
ciphertext without decrypting. Switching to generic drops the token, pixel
id and test code; a platform provider drops the URL.

### UI

The pixel form shows the fields of the chosen provider (pixel id, token —
write-only, "Saved — ends in …", remove checkbox —, test code; or the URL
with the macro picker) and an event name per selected event with the
platform default as placeholder. The pixel page has a **Deliveries** card:
the last 100 deliveries with status, attempts, HTTP code and the
platform's error message, refreshed every 15 s.

### Not done

- Twitter/X sending (see above).
- A "send test event" button (today: set the test event code and send a
  test conversion through the funnel).
- Browser-side pixels (`fbq` on landings): FLOX's own landings could load
  them, but server-side covers every landing type and needs no page change.

---

# History: the CRUD phase

Fourth slice of the Landing/PWA/Postlanding/Pixels CRUD candidate list,
after [Postlanding](postlanding.md). `pixels` was already migrated
(migration 00008, alongside `postbacks` and the `stream_set_pixels` join
table), flat, no children — same shape/precedent as Postlanding.

## Backend: mirrors `apps/internal/postlanding` almost exactly

`apps/internal/pixel`: `model.go`/`handler.go`/`service.go`/
`repository.go`, wired at `/pixels` in `apps/api/main.go` the same way
`/postlandings` is. No URL at all, unlike Landing/PWA/Postlanding: a
pixel's `provider` + `pixelId` identify where a conversion gets reported,
not a page a visitor is sent to.

Validation in `pixel.Service`:
- `name`: 2-100 chars.
- `provider`: one of `facebook`/`tiktok`/`snapchat`/`twitter`/`generic`
  (`Provider.Valid()`).
- `pixelId`: free-form string, at most 80 chars — **not required**.
  Matches the frontend's own pre-existing zod schema
  (`z.string().max(80)`, no `.min(1)`): an operator can save a pixel
  before the provider has issued a real id yet.
- `events`: at least one event required, every value must be one of
  `ValidEventTypes` — a curated 6-entry subset of the full §43 event
  model (`PWA_INSTALL`, `CPA_HOLD`, `CPA_ACCEPT`, `CPA_REDEP`, `TG_JOIN`,
  `NOTIFICATION_SUBSCRIBE`), matching the frontend's
  `lib/api/pixels.ts`'s `PIXEL_EVENT_TYPES` exactly — checked for exact
  parity, same precedent as Postlanding's own curated subset.

`Create`/`Update`/`Delete`/`Duplicate` (copies fields verbatim, appends
" (Copy)", preserves non-active status via a follow-up `Update`)/`Pause`/
`Activate` (idempotent, `apierror.Conflict` from archived state) all
follow the same shape as `postlanding.Service`.

## Delete: no defensive FK catch needed, unlike Landing/PWA/Postlanding

`stream_set_pixels.pixel_id` (migration 00008) is the one place a pixel
gets referenced, and it `ON DELETE CASCADE`s from `pixels` — the
opposite of `flows.landing_id`/`pwa_id`/`postlanding_id`'s `RESTRICT`.
Deleting a pixel just drops its stream-set attachments along with it, so
`pixel.Repository.Delete` has no `23503` catch to add, unlike its three
siblings' `Delete` methods.

## Frontend

New `lib/api/pixels.ts` + `hooks/use-pixels.ts` (real API layer, mirrors
`lib/api/postlanding.ts`/`hooks/use-postlandings.ts` almost exactly).
`pixel-list.tsx`/`-form-sheet.tsx`/`-columns.tsx`/`-row-actions.tsx`
rewired off `stores/pixels.ts` (Zustand mock) onto the real hooks;
`LoadingState`/`ErrorState` added. No Content Gallery integration exists
for Pixels (unlike Landing/PWA/Postlanding) — the pre-existing mocked
`pixel-list.tsx` never had one either, so there was nothing to carry
forward.

`stores/pixels.ts` and `lib/mock/pixels.ts` deleted outright once
grepping confirmed zero remaining importers.

Provider display labels moved into `PIXEL_PROVIDER_I18N_KEY` (co-located
with `PixelProvider` in `lib/api/pixels.ts`), same pattern as
`trafficSources.ts`'s `SOURCE_TYPE_I18N_KEY` — the wire/stored value is
always the raw provider string; only the rendered label goes through
`t()`. Event codes (`PWA_INSTALL`, `CPA_HOLD`, ...) in the multi-select
stay deliberately untranslated — canonical §43 identifiers, not UI text,
same treatment as Postlanding's own event picker.

### i18n: added, not skipped

Same reasoning as every prior real-backend phase this session: Pixels
was still mocked (and, per its own mock file's doc comment, entirely
untranslated — hardcoded English strings) when the Frontend i18n phase
ran. Added a new `pixels` namespace (`en`+`ru`, both complete — key-set
parity checked directly) and registered it in `lib/i18n/config.ts`'s
`NAMESPACES`.

## Per-Stream-Set Pixel attachment: wired in a later phase

At the time this phase closed, `stream_set_pixels` (migration 00008,
the join table a Stream Set uses to fire zero or more of these Pixels)
had no CRUD wiring it to `stream_sets`. That landed in its own follow-on
phase — see `docs/stream-sets.md`'s "Stream Set ↔ Pixel attachment"
section. `docs/stream-sets.md`'s own past "per-flow Pixels" phrasing was
imprecise: the schema has always scoped pixels to the *Stream Set*, not
the Flow.

## Verified

- Backend: `go build/vet/gofmt/test ./...` all green — new `pixel_test.go`
  mirrors `postlanding_test.go`'s full test set (create/get/update/delete,
  an empty `pixelId` explicitly allowed, invalid-shape validation incl.
  bad provider/zero events/unrecognized event type/short name,
  pause/activate transitions incl. idempotency and archived-state
  conflict, duplicate keeps status, cross-tenant isolation across
  get/update/delete/list).
- Frontend: `tsc --noEmit`/`eslint`/`vitest run` (21 tests, unchanged —
  this phase's tests are backend-only, mirroring an existing CRUD shape
  with no new frontend logic to unit-test)/`next build` (production) all
  clean.
- Full manual browser pass against the real running `api`+`web` dev
  servers: confirmed the real empty state (no leftover mock seed data,
  unlike the old store's 3 hardcoded pixels); created a pixel (provider
  select showing real translated labels, one event selected via the
  multi-select); reloaded fresh and confirmed the row round-tripped
  through the real API (name/provider/pixelId/events/status all
  correct); paused it (status badge updated live); duplicated it (copy
  kept the paused status, provider, pixelId, and events verbatim);
  edited it and confirmed every field pre-filled correctly; archived it
  (confirmation dialog interpolated the name correctly, toast confirmed,
  status badge updated to "Archived", action menu correctly dropped to
  Edit/Duplicate only). Also spot-checked the Russian locale on this
  page (sidebar label, title, column headers, status badges, button —
  all correctly translated, no hydration errors). Test pixel rows
  deleted directly from Postgres afterward (no hard-delete in the UI for
  this entity, same as Landing/PWA/Postlanding).

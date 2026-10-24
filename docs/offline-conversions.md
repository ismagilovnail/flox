# Offline conversions to Google Ads and Yandex Metrica

Audit UNI-2 part 2. A traffic source connected to Google Ads
(docs/ad-account-connections.md) can report its leads, purchases and
repeat purchases back to Google, by the click id Google put on the click
(`gclid`, or `gbraid`/`wbraid` for iOS app-to-web clicks — docs/attribution.md
"Search and ad networks' click ids"). Google then sees which ads bring
sales and can bid for them.

## Why the Data Manager API

Google's own words: from 15 June 2026, `UploadClickConversions` fails for a
developer token that has not uploaded offline conversions before; new
integrations use the Data Manager API. FLOX uses
`POST https://datamanager.googleapis.com/v1/events:ingest`
(`internal/adaccount/googleads/datamanager.go`). It is written against the
documented request and tested against a fake server only: there is no Google
Ads account here.

What the operator needs on Google's side:

- a conversion action with the source "Import from clicks" (type
  `UPLOAD_CLICKS`) per status to send; its numeric id goes in the settings;
- the Data Manager API enabled in the Google Cloud project of the OAuth client;
- the source's refresh token minted with the
  `https://www.googleapis.com/auth/datamanager` scope (with the Ads scope too,
  so cost sync keeps working) by a Google account with access to the ad
  account, or to the manager account named as the login account.

## Settings (per source)

`GET/PUT /v1/traffic-sources/{id}/connection/offline-conversions`
(`internal/offlineconv`, migration 00104; not in the public API, like the rest
of `/connection`):

| Field | Meaning |
|---|---|
| `enabled` | on/off. Turning it on starts the window (`since`): only conversions recorded from then on are sent; nothing older is backfilled |
| `leadActionId` / `purchaseActionId` / `repurchaseActionId` | the conversion action for CPA_HOLD / CPA_ACCEPT / CPA_REDEP; empty: that status is not sent. At least one is needed to turn it on |
| `loginAccountId` | the manager account (MCC) the credentials reach the ad account through; empty: direct access |

The answer also carries what came of it: `counts` per status, `skipped` per
reason, Google's `lastError`. The source's page shows them under the
connected account.

## What is sent

apps/worker runs every 5 minutes:

1. **Enqueue.** Each new successful CPA_HOLD/ACCEPT/REDEP postback of an
   organization with the export on gets one row in
   `offline_conversion_uploads` (unique per postback):
   - `pending` when its click is known, its campaign's traffic source is the
     connected Google Ads source with the export on since before the
     postback, the click carried `gclid`/`gbraid`/`wbraid`, and the status
     has an action;
   - `skipped` otherwise, with the reason: `not_attributed`,
     `click_not_found`, `source_off`, `no_google_click`, `no_action`.
     A lookup that fails (ClickHouse down) records nothing and is tried again.
2. **Restate.** A refund (docs/conversion.md "Refunds by order") leaves
   less of a purchase: its row goes back to `pending` with the new value.
3. **Send.** Pending rows that are due, per source and action, in requests of
   at most 2000 events. Each event: the click id, the conversion time
   (RFC 3339), `transactionId` = the postback id, `conversionValue` and
   `currency` as the network reported them (no USD conversion: Google
   converts), `eventSource` WEB. Sending a transaction id again with another
   value restates it, per Google's guide ("conversion adjustments"); a full
   refund restates it to 0 — Google documents no separate retraction for this
   API.

Outcomes:

- taken (2xx): `sent`, with Google's request id and field warnings;
- 400: Google refuses the whole request for one bad event. A request of
  several is sent again one event at a time; the bad event is `failed` with
  Google's message, the others go;
- anything else (a refused refresh token, a missing scope or access, 429,
  5xx, the network): stays `pending`, tried again after 1, 2, 4… minutes up
  to 6 hours; `failed` after 12 attempts (about two days).

A row being sent is leased for 10 minutes (`FOR UPDATE SKIP LOCKED`), so two
workers do not send it twice. A refund that arrives while it is in flight
leaves it pending for the new value.

`flox_offline_conversions_total{result=sent|retry|failed}`.

## Not done

- **Consent.** No `consent` is sent: FLOX does not know the visitor's
  `ad_user_data` consent. Google may not attribute EEA conversions without
  it.
- **Enhanced conversions** (hashed email/phone) are not sent.
- **Not checked against the live API**: the request shape, the per-event
  error format, restating to 0. The first real account should be tried with
  one conversion before turning it on for a source.
- Microsoft Ads (msclkid is stored, not sent).

# Yandex Metrica

Audit UNI-2 part 3. Any traffic source (its cost integration does not
matter: Direct's cost import is UNI-3) can connect the Metrica counter on
its landing. Conversions of its clicks that came from Yandex Direct with a
`yclid` are uploaded to the counter; Metrica attaches them to the visit
that `yclid` started and passes them on to Direct.

Written against Metrica's documented Management API
(`internal/yandexmetrica`; yandex.ru/dev/metrika management/offline-conv,
openapi offline_conversions upload/findById) and tested against a fake
server only: there is no counter here.

What the operator needs on Metrica's side:

- a goal of the type "JavaScript event" (condition "matches") per status to
  send; its identifier goes in the settings;
- an OAuth token (Yandex OAuth, with `metrika:offline_data` or
  `metrika:write`) of a Yandex account with access to the counter. Pasted,
  like the ad account tokens (docs/ad-account-connections.md "Why manual
  token paste");
- **the `yclid` on the landing's address**: Metrica matches the visit it
  recorded with that `yclid`. Direct puts it on the tracking link; FLOX
  redirects, so the offer or landing URL must pass it on: `yclid={yclid}`
  (docs/attribution.md macros).

## Settings (per source)

`GET/PUT/DELETE /v1/traffic-sources/{id}/connection/metrica`
(`internal/offlineconv/metrica.go`, migration 00105, table
`metrica_connections`; not in the public API, like the rest of
`/connection`). GET of a source with no counter answers `connected: false`.

| Field | Meaning |
|---|---|
| `counterId` | the counter's number |
| `token` | write only; empty on a later save keeps the stored one. Stored AES-256-GCM encrypted under `ADACCOUNT_ENCRYPTION_KEY`; only its last 4 characters (`tokenPreview`) come back |
| `currency` | the counter's currency (default RUB) |
| `enabled` | on/off; turning it on starts the window (`since`), nothing older is sent |
| `leadGoal` / `purchaseGoal` / `repurchaseGoal` | the goal for CPA_HOLD / CPA_ACCEPT / CPA_REDEP; empty: not sent. At least one to turn it on |

A new token or counter is checked with Metrica on save
(`GET /management/v1/counter/{id}`): a refused token or a counter Metrica
does not know is a field error; Metrica not answering is a 503.

## What is sent

The same queue and loop as Google (`offline_conversion_uploads`, apps/worker
every 5 minutes); a row is Google's (`connection_id`) or Metrica's
(`metrica_connection_id`). The click's own id decides: a `yclid` click goes
to the source's Metrica connection, a Google one to its Google connection.
Skip reasons besides Google's: `no_yandex_click` (a source exporting only to
Metrica, a click without `yclid`) and `visit_too_old` (the click is older
than 21 days, Metrica's window for offline data).

- **The file.** Per counter, up to 5000 rows a file: CSV
  `Yclid,Target,DateTime,Price,Currency`, DateTime in Unix seconds, posted
  as the multipart field `file` to
  `POST /management/v1/counter/{id}/offline_conversions/upload` with
  `Authorization: OAuth <token>`. Every goal of the counter goes in one
  file.
- **Money.** Metrica zeroes a price in another currency than the counter's,
  so the value (what is left after refunds made before the upload) is
  converted to the counter's currency at the rates of the event's date
  (`fx_rates`, as everywhere — CLAUDE.md #7). No rate: the row goes without
  a price, never as 0. A lead without revenue goes without a price.
- **No restating.** Metrica's API has no change or cancel of an uploaded
  conversion; sending the row again would count a second purchase. A
  refund after the upload stays in Metrica (Google's rows are restated).
- **Outcomes.** 2xx: `sent`, `request_id` = Metrica's upload id,
  `remote_status` = its status; fewer valid rows than sent is noted as a
  warning. 400 (an invalid file): the file is split in halves until the bad
  row is alone and `failed` (halves, not row by row: Metrica allows 5000
  requests a day per user). A refused token or access, 429, 5xx, the
  network: retried like Google's.
- **Check.** Metrica matches a file within about 3 hours. Each run, files
  sent in the last 2 days that are not final, last looked at 30+ minutes
  ago, are read back (`GET …/offline_conversions/uploading/{id}`):
  `PROCESSED` is final; `LINKAGE_FAILURE` (no row matched a visit) fails
  the file's rows with an explanation (counter on the landing, `yclid` in
  its address, the goals in the counter). Metrica does not say which rows of
  a processed file matched.

The source's page shows the counter, the token's last 4 characters, the
goals, the counts (sent, being matched, queued, refused or unmatched), skip
reasons and Metrica's last error.

## Not done (Metrica)

- **Not checked against a live counter**: the file format as Metrica reads
  it, the statuses.
- Identifiers other than `yclid` (Metrica's ClientId, UserId): FLOX does
  not know the visitor's Metrica ClientId.
- Calls, CRM orders (Metrica's other offline data).

# Offline conversions to Google Ads

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
- Yandex Metrica (UNI-2 part 3); Microsoft Ads (msclkid is stored, not sent).

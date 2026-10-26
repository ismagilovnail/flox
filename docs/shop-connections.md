# Shop connections: Shopify, WooCommerce, Tilda (audit UNI-4 parts 3–5)

An own-site conversion source (docs/networks-offers.md "Own site") whose
site is a Shopify, WooCommerce or Tilda store reports its orders by webhook
instead of calling the postback URL: paid orders become purchases and
repeat purchases, refunds take back what was paid back. One
`shop_connections` row per source and platform (`internal/shopconn`: the
settings, the order lookups; `internal/shopify`, `internal/woocommerce`,
`internal/tilda`: the webhook formats; apps/tracker `shop.go`: what both record). On the
source's page: the "Online store" card, a tab per platform.

## Shopify

Written against shopify.dev (webhook delivery and HMAC verification, the
orders/paid and refunds/create payloads, the Cart AJAX API) and the Help
Center (webhooks created in the admin). Tested against signed payloads on
the real Postgres and conversion engine; **not tried with a real store**.

## Setting it up

On the source's page, the "Online store" card, Shopify tab (own sites only):

1. **Store domain** — `my-shop.myshopify.com` (`https://`, a path and the
   case are dropped). A webhook whose `X-Shopify-Shop-Domain` is another
   store is refused, even when signed with the same key.
2. **Signing key** — the key Shopify signs webhooks with: for webhooks
   created in the admin (Settings › Notifications › Webhooks) the key that
   page shows; for a custom app, its client secret. The Help Center only
   says the admin's webhooks are "signed with an ID … unique to your
   store": check with the admin's "Send test" before relying on it (a wrong
   key answers 401). Stored AES-256-GCM encrypted under
   `SHOP_WEBHOOK_ENCRYPTION_KEY` (docs/environment.md); only its last 4
   characters come back.
3. **Two webhooks** to the card's URL (`TRACKER_PUBLIC_URL/shopify/{id}`),
   format JSON: "Order payment" (`orders/paid`) and "Refund create"
   (`refunds/create`).
4. **The theme tag** on every page (theme.liquid, before `</head>`):
   `<script src="TRACKER/flox.js" async></script>`.

`GET/PUT/DELETE /v1/networks/{id}/shopify` (`internal/shopconn`, migration
00108; not in the public API). A source that is not an own site is refused:
the webhooks are recorded as its `purchase` / `repeat_purchase` / `refund`
statuses.

## Which click an order is

1. The cart attribute `flox_click_id`: on a Shopify storefront
   (`window.Shopify.shop`) flox.js posts the click id it keeps
   (docs/networks-offers.md "Own site without a server") to
   `cart/update.js` as an attribute, once per click and session. The order
   keeps cart attributes as `note_attributes`. (The pages read do not state
   that mapping in words; it is Shopify's long-standing behaviour.)
2. Else `click_id` in the order's `landing_site`: an offer link straight to
   the store carries it, but only the visit that started the session counts.
3. Neither: the order is still sent to the engine without a click, so it
   shows in the postback log as refused.

## What is recorded (apps/tracker `POST /shopify/{networkId}`)

The raw body is read (≤ 2 MB) and `X-Shopify-Hmac-SHA256` checked — base64
HMAC-SHA256 of the body with the key — before anything else. Wrong
signatures and unknown sources count toward the per-IP refusal limit, as
for postbacks.

- **orders/paid** → `purchase` with `txn_id` = the order id, `revenue` =
  `total_price` in `currency` (the shop's), `event_time` = `processed_at`.
  The click already has a purchase → `repeat_purchase` (its `txn_id` makes
  each order its own). Two orders of one click racing: the purchase comes
  back a duplicate and is sent again as a repeat purchase — unless the
  order is now recorded (the same order delivered twice at once). The click
  is looked up before the order, so such a twin always ends a duplicate
  (`TestShopOrderRacingItself`).
- **A replay** (Shopify may deliver a webhook twice): an order id already
  recorded as a purchase or repeat purchase of the source is answered
  `duplicate` without calling the engine — otherwise the replay of a first
  order would become a repeat purchase. Index: migration 00109.
- **refunds/create** → `refund` of that order (`txn_id` = order id, the
  click from the recorded purchase), `refund_amount` = the refund's
  successful `refund` transactions — what was actually paid back —,
  `refund_id` = the refund's id, so its replay is a duplicate and two
  partial refunds are not (docs/conversion.md "Refunds by order"). A
  transaction currency other than the purchase's is refused by the engine.
- Left with 200: test orders (`test: true`), a refund that paid nothing
  back, a refund of an order FLOX did not record, any other topic.

Shopify waits 5 seconds, then retries a non-2xx answer 8 times over 4 hours
and may delete the subscription. So everything the engine decided —
recorded, duplicate, refused with its reason in the postback log — is a 200;
only FLOX's own faults (database, a missing key: 503) are not.

`flox_shop_webhooks_total{platform, topic, result}` (topic bounded to the
ones used and `other`).

### Not done (Shopify)

- Not tried with a real store (signing key of admin webhooks, the cart
  attribute reaching the order on every checkout).
- `orders/cancelled` is not read: a cancelled paid order is refunded, and
  the refund is.
- Multi-currency stores: amounts are taken in the shop's currency; a refund
  transaction in the buyer's currency is refused (visible in the log).

## WooCommerce (audit UNI-4 part 4)

Written against the WooCommerce docs and source (webhooks: delivery
headers, the signature, the activation ping, failure handling; the REST
API v3 order shape the webhooks carry; order attribution). Tested against
signed payloads on the real Postgres and conversion engine and on the dev
tracker; **not tried with a real store**.

### Setting it up

The "Online store" card, WooCommerce tab:

1. **Site address** — WordPress › Settings › General, e.g.
   `shop.example.com` or `example.com/shop` (scheme, case and trailing
   slash dropped, a path kept). WooCommerce sends it as
   `X-WC-Webhook-Source` (`home_url("/")`); a webhook from another site is
   refused even when signed with the same secret.
2. **Webhook secret** — WooCommerce › Settings › Advanced › Webhooks: the
   Secret field of the webhooks (the same on both). Stored like Shopify's
   key (`SHOP_WEBHOOK_ENCRYPTION_KEY`, last 4 characters shown).
3. **Two webhooks** to the card's URL (`TRACKER_PUBLIC_URL/woocommerce/{id}`),
   status Active, API version "WP REST API Integration v3": topics
   "Order created" and "Order updated". Both: a checkout paid in the same
   request may be delivered as either (the payload is built when it is
   sent).
4. **The site tag** on every page (`<script src="TRACKER/flox.js" async>`,
   in the theme's `<head>` or a header-scripts plugin): flox.js keeps the
   click in the first-party `flox_click_id` cookie.
5. **The checkout snippet** (card, copyable; Code Snippets plugin or the
   theme's functions.php): it copies that cookie into the order's
   `flox_click_id` meta field, on `woocommerce_checkout_order_created`
   (classic checkout) and `woocommerce_store_api_checkout_order_processed`
   (block checkout) — before the payment is taken, so the payment's update
   carries it.

`GET/PUT/DELETE /v1/networks/{id}/woocommerce` (migration 00110; not in
the public API).

### Which click an order is

1. The order meta field `flox_click_id` (the snippet).
2. Else `click_id` in the order meta `_wc_order_attribution_session_entry`:
   WooCommerce's order attribution (8.5+, on by default) keeps the page the
   visit entered by; an offer link straight to the store carries it.
3. Neither: the order still goes to the engine without a click and shows
   in the postback log as refused.

### What is recorded (apps/tracker `POST /woocommerce/{networkId}`)

- **The ping** WooCommerce sends when a webhook is made active — an
  unsigned form body `webhook_id=N` — is answered 200 (anything else shows
  the owner a delivery error). It records nothing. The source must already
  have its WooCommerce store connected (else 404).
- `X-WC-Webhook-Signature` — base64 HMAC-SHA256 of the raw body with the
  secret — is checked before anything else; refusals count toward the
  per-IP limit, as for postbacks.
- **order.created / order.updated** carry the whole order, with every
  update of it:
  - not paid (`date_paid_gmt` empty, or status pending / failed /
    cancelled / checkout-draft) → `ignored_unpaid`. WooCommerce sets
    `date_paid` when the payment completes — for cash on delivery when the
    order is completed — and keeps it through a refund;
  - paid → a `purchase` (`txn_id` = the order id, `revenue` = `total`,
    `currency`, `event_time` = `date_paid_gmt` as UTC), or a
    `repeat_purchase`, exactly as for Shopify; every later update of the
    order is a `duplicate` without calling the engine;
  - each entry of `refunds` (oldest first; its `total` is negative) →
    a `refund` of that amount keyed by the refund's id, in the order's
    currency. A refund already recorded is skipped before the engine, so
    the next updates do not log a duplicate per refund. WooCommerce has no
    refund webhook: a refund arrives as an update of its order.
- Other topics → 200, left.

WooCommerce does not retry a failed delivery and disables a webhook after
5 failures in a row: as for Shopify everything the engine decided is a
200; a fault on FLOX's side (database, no key: 503) loses that delivery —
the next update of the order brings the payment and refunds again.

### Not done (WooCommerce)

- Not tried with a real store: the two checkout hooks, the attribution
  meta coming through the REST payload, the ping without a signature.
- A paid order that is later cancelled without a refund is not reversed
  (a cancellation is not money paid back).
- Multi-currency plugins: amounts are taken in the order's currency.

## Tilda (audit UNI-4 part 5)

Written against Tilda's help pages (help-ru.tilda.cc/forms/webhook: the
POST of form fields, `tranid`/`formid`, `COOKIES`, the API key, the test
request, 5 s and two retries a minute apart). The cart's order fields are
**not documented** there or on the cart and payment pages: FLOX reads the
`payment` block in the shapes it is known to arrive in (`payment[orderid]`,
`payment[amount]` form fields; `payment` as a JSON string; a JSON body)
and has to be checked with a real order. Tested with keyed payloads on
the real Postgres and conversion engine and on the dev tracker; **not
tried with a real site**.

### Setting it up

The "Online store" card, Tilda tab:

1. **API key** — Tilda does not sign webhooks; this key is the check.
   "Generate a key" makes one (shown in clear: it has to be copied into
   Tilda). Stored like the others (`SHOP_WEBHOOK_ENCRYPTION_KEY`).
2. **Site address** — optional, the owner's note: a Tilda webhook does not
   say which site sent it, so nothing is compared.
3. **The receiver** — Site settings › Forms › Webhook: the card's URL
   (`TRACKER_PUBLIC_URL/tilda/{id}`), API Method "POST" (or headers), API
   Name `floxkey`, API Key the key, **"Send cookies" ticked**; then attach
   the receiver to the forms and the cart.
4. **The site tag** — Site settings › More › HTML code for the HEAD:
   flox.js keeps the click in the `flox_click_id` cookie, which Tilda sends
   in `COOKIES`.
5. **The cart** must send to receivers only after a successful payment
   (check the cart's / payment system's setting): FLOX cannot tell a paid
   order from an unpaid one.

`GET/PUT/DELETE /v1/networks/{id}/tilda` (migration 00111).

### What is recorded (apps/tracker `POST /tilda/{networkId}`)

- The connect test (`test=test`, with the key or without) → 200, nothing
  recorded.
- The key — header or POST field `floxkey`, compared in constant time — is
  checked first; a missing or wrong one is a 401 and counts toward the
  per-IP refusal limit.
- The click: `flox_click_id` in `COOKIES`, else a form field `click_id`
  (a hidden field; Tilda capitalizes it to `Click_id` unless a variable
  name is set) or `flox_click_id`.
- A submission with an order (`payment` with an `orderid`) → `purchase`,
  or `repeat_purchase` when the click already bought, `txn_id` = the order
  id, `revenue` = `amount`, the source's currency unless the payment names
  one; a retry is a duplicate before the engine (as for Shopify).
- Any other form → `lead` (CPA_HOLD), `txn_id` = `tranid`; a retry or a
  second form of the click is a duplicate (the engine's key).
- Everything the engine decided is a 200: Tilda retries twice, a minute
  apart, a request that does not answer in 5 seconds.

### Not done (Tilda)

- Not tried with a real site: the cart's field names and when the cart
  sends (before or after payment).
- No refunds or cancellations: Tilda does not send them.
- Every form attached to the receiver is a lead — attach it only to the
  forms that are leads.

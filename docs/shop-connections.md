# Shop connections: Shopify (audit UNI-4 part 3)

An own-site conversion source (docs/networks-offers.md "Own site") whose
site is a Shopify store reports its orders by webhook instead of calling
the postback URL: paid orders become purchases and repeat purchases,
refunds take back what was paid back. WooCommerce is planned the same way
(`shop_connections.platform`).

Written against shopify.dev (webhook delivery and HMAC verification, the
orders/paid and refunds/create payloads, the Cart AJAX API) and the Help
Center (webhooks created in the admin). Tested against signed payloads on
the real Postgres and conversion engine; **not tried with a real store**.

## Setting it up

On the source's page, the "Shopify store" card (own sites only):

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

`GET/PUT/DELETE /v1/networks/{id}/shopify` (`internal/shopify`, migration
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
  each order its own). Two orders racing: the purchase comes back a
  duplicate and is sent again as a repeat purchase.
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
two used and `other`).

## Not done

- Not tried with a real store (signing key of admin webhooks, the cart
  attribute reaching the order on every checkout).
- `orders/cancelled` is not read: a cancelled paid order is refunded, and
  the refund is.
- Multi-currency stores: amounts are taken in the shop's currency; a refund
  transaction in the buyer's currency is refused (visible in the log).
- WooCommerce (UNI-4 part 4).

# Telegram bots — TG_START, TG_JOIN

§43's TELEGRAM events, produced by an organization's **own** bot. Scope
chosen by the owner on 2026-10-14: FLOX takes the bot's webhook; a join of
the bot's channel is attributed through the user's `/start` (no per-click
invite links); pixels for TG events came next (2026-10-15, `docs/pixels.md`).

## Flow

```
click /t/{slug} ──► destination https://t.me/<bot>?start={click_id}
   SOURCE_CLICK          │
                         ▼ user taps Start
               Telegram ──► POST tracker /tg/{botId}  "/start <click_id>"
                              TG_START on that click; reply: welcome + channel button
                         ▼ user joins the channel (or asks to; the bot approves)
               Telegram ──► POST tracker /tg/{botId}  chat_member: left → member
                              TG_JOIN on the click of that user's /start
```

The destination is an ordinary URL with the `{click_id}` macro — the bot
link Settings shows (`deepLink`) — so no routing change was needed.

## Connecting (apps/api, `internal/telegram`)

Settings → Integrations → **Telegram bots** (`settings.write`):
`GET/POST /telegram-bots`, `DELETE /telegram-bots/{id}`. Connecting:

1. `getMe` with the token (malformed or refused: a `token` field error
   carrying Telegram's own reason).
2. With a channel (`@name` or `-100…`): `getChat` (a channel or supergroup),
   `getChatMember` — the bot must be an **administrator that can invite
   users**; the invite link is `t.me/<name>` for a public channel, else one
   `createChatInviteLink` made now.
3. `setWebhook` to `TELEGRAM_WEBHOOK_BASE_URL/tg/{id}` with a random
   secret (only its SHA-256 is stored) and `allowed_updates` = message,
   chat_member (Telegram leaves it out unless named), chat_join_request.
4. Stored: the token encrypted with `INTEGRATION_ENCRYPTION_KEY`, never
   returned. A bot already connected (here or by another organization —
   one bot has one webhook) is 409, and the webhook just set is removed.

Removing a bot deletes its webhook (best effort — a revoked token cannot)
and its rows. Telegram unreachable: 503, and the error never carries the
token (it is in the Bot API URL; transport errors are unwrapped).
Without `TELEGRAM_WEBHOOK_BASE_URL` the panel says so and Connect is off.

## The webhook (apps/tracker)

`POST /tg/{botId}` — authenticated by Telegram's
`X-Telegram-Bot-Api-Secret-Token`, compared in constant time; wrong or
missing: 404. It never calls Telegram: an answer (the welcome message, a
join approval) is returned in the HTTP response, which Telegram executes.

- **/start <click_id>** in a private chat: the click is looked up in the
  bot's organization (`attribution.ClickResolver`, ClickHouse) — another
  organization's or an unknown click records nothing. `telegram_users`
  keeps (bot, Telegram user) → click; TG_START once per click (the same
  link tapped again is not a second start; the same person from another ad
  is). The event carries the click's context (`attribution.Click.CopyTo`,
  shared with conversions): campaign, flow, offer, geo, device, subs.
  Every /start is answered with the welcome text and a button to the
  channel.
- **chat_member** of the bot's channel, not a member → member: TG_JOIN on
  the click of that user's last /start, once per click. Joined without a
  /start: nothing (no click to attribute it to — FLOX does not guess).
- **chat_join_request** to the bot's channel: approved; the chat_member
  update that follows records the join.
- ClickHouse not answering: 503, nothing marked, so Telegram re-sends the
  update. An update that cannot be parsed: 200 (a retry would not help).

The route is listed in the api's `deliberatelyOpen` (no session — the
secret authenticates it).

## Not done

Per-click invite links
(joins without /start); several channels per bot; editing a bot (remove and
connect again).

## Pixels

A pixel can report TG_START and TG_JOIN (`pixel.ValidEventTypes`): the
worker's `pixelsend.TriggerSink` queues them like PWA_INSTALL, for the
pixels of the click's stream set. The webhook is called by Telegram, so the
visitor's IP and user agent sent to the platform are the click's. Default
names: Facebook/TikTok `Contact` / `Subscribe`, Snapchat `CUSTOM_EVENT_1` /
`SUBSCRIBE`.

## Verified (2026-10-14)

- Tests: `internal/telegram` against a fake Bot API — connect (public and
  private channel, webhook URL/secret/allowed updates, token stored
  encrypted, list order), refusals (malformed/unknown token, channel shape,
  missing, not a channel, bot not admin, welcome too long, unconfigured,
  unreachable without leaking the token, already connected → 409 and
  webhook removed), cross-tenant list/delete, delete removes the webhook;
  the webhook (secret, start with context, start once, other organization's
  and unknown clicks, no payload, group chat, join once, join without start,
  other chat, promotion is not a join, join request approved, a second ad,
  garbage acknowledged, resolver down → 503 with nothing stored, unknown bot).
  Mutation-checked: dropping the secret comparison, the start dedup or the
  join dedup each fails a test. The api's RBAC and isolation guards cover
  the new routes.
- Live on the dev stack: a bot row with a known secret, Telegram-shaped
  updates posted to the running tracker → wrong secret 404, /start answered
  with the welcome and the channel button, the join request approved,
  TG_START and TG_JOIN in ClickHouse on a real dev click with its campaign
  and OS. In Chrome: the panel (not-configured notice, the bot with its
  channel and deep link, remove with confirm → empty state). The connect
  dialog itself was not exercised in the browser: the dev deployment has no
  public webhook URL.

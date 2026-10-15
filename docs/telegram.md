# Telegram bots — TG_START, TG_JOIN

§43's TELEGRAM events, produced by an organization's **own** bot. Scope
chosen by the owner on 2026-10-14: FLOX takes the bot's webhook; a join of
the bot's channel is attributed through the user's `/start`; pixels for TG
events came next (2026-10-15, `docs/pixels.md`), then per-click invite
links, so a join without /start is attributed too (2026-10-15, below).

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

## Per-click invite links

To send visitors straight to the channel, use the bot's **channel link per
click** (Settings, shown for a bot with a channel on a configured
deployment) as the destination:
`TELEGRAM_WEBHOOK_BASE_URL/tgj/{botId}?click={click_id}`.

```
click /t/{slug} ──► /tgj/{botId}?click={click_id}  (tracker)
   SOURCE_CLICK          claims a free link of the pool for the click ──► 302 t.me/+…
                         ▼ user joins through it
               Telegram ──► POST /tg/{botId}  chat_member with invite_link
                              TG_JOIN on that link's click (no /start needed)
```

- **Pool** (`telegram_invite_links`, migration 00077). Telegram creates
  invite links and the redirect may not call it (CLAUDE.md #9), so the
  worker (`telegram.InvitePool`, every minute) keeps `InvitePoolSize` (50)
  free links per bot with a channel, at most 20 new per bot per run
  (`createChatInviteLink`, named "FLOX click"). It needs
  `INTEGRATION_ENCRYPTION_KEY` (the bots' tokens) and `TELEGRAM_API_URL` if
  set. A bot Telegram refuses (rights taken away, token revoked) is logged
  and skipped; the others go on.
- **Claim** (`GET /tgj/{botId}?click=`, tracker): one Postgres statement —
  the click's link if it already has one (a reload gets the same link),
  else the oldest free one, `FOR UPDATE SKIP LOCKED` so two clicks never
  share one. No Telegram call. An empty pool, or no/invalid click id
  (the template opened as is), redirects to the channel's shared link: the
  visitor gets in, the join counts only through a /start;
  `flox_telegram_invite_fallback_total` counts the empty-pool case. Unknown
  bot or one without a channel: 404. `Cache-Control: no-store`.
- **Join.** `chat_member` carries the link the user joined through
  (`invite_link`, in full for links the bot created). A claimed link of
  this bot: TG_JOIN on its click, once per link (`joined_at`). The link
  already used (passed on to someone else), or not one of ours (the shared
  link): the /start path as before. The link's click not in ClickHouse yet
  and claimed under 10 minutes ago: 503, Telegram re-sends the update;
  later: nothing recorded.
- **Expiry.** Claimed links are revoked (`revokeChatInviteLink`) and
  forgotten after `InviteLinkTTL` (30 days), up to 100 per bot per run; a
  link Telegram refuses to revoke is forgotten too, one it could not be
  asked about is kept for the next run.
- Settings shows the link with a copy button and the free links left
  (`freeInviteLinks`; 0 is shown as a warning).

The click id in `/tgj/` is not signed, like the `/start` payload: anyone can
claim links with made-up ids. That only spends the pool (refilled each
minute; meanwhile the shared link) — a made-up click is not found when its
join comes, so nothing is recorded.

## Not done

Several channels per bot; editing a bot (remove and connect again);
revoking a removed bot's unclaimed pool links (its token is gone with it —
they stay valid invites to the channel, attributed to nothing).

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

## Verified (2026-10-15, per-click invite links)

- Tests (`invites_test.go`, fake Bot API): the pool filled in runs of 20 up
  to 50; a click's own link, the same on a reload, another for another
  click; shared link with an empty pool, without a click id or with the
  macro unresolved; unknown bot 404; 20 concurrent clicks, 20 different
  links; a join through the link without /start is TG_JOIN with the click's
  context, once; the link passed on falls back to /start; a join before
  the click is stored is 503, an hour later 200 with nothing; revocation
  of a link past its TTL only, top-up after it, a scoped run leaving another
  organization's bot alone, a refusing bot not failing the run.
  Mutation-checked: dropping the row lock (a link handed out twice), the
  reuse of a click's link, the pending-click retry, the invite path itself,
  and both join guards together each fail a test (each guard alone is
  covered by the other).
- Live on the dev stack: a bot row with two pool links, a real dev click →
  `/tgj/` 302 to the first link (the same on a second request), without a
  click to the shared link, unknown bot 404; a Telegram-shaped `chat_member`
  with that `invite_link` sent twice → one TG_JOIN in ClickHouse on the
  click with its campaign and OS; the running worker logs and skips a bot
  whose token cannot be decrypted. The panel's new row was not seen in the
  browser: the dev deployment is not configured for Telegram (no
  `TELEGRAM_WEBHOOK_BASE_URL`), so the API leaves `clickInviteLink` out.

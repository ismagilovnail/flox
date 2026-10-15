# Telegram bots — TG_START, TG_JOIN

§43's TELEGRAM events, produced by an organization's **own** bot. Scope
chosen by the owner on 2026-10-14: FLOX takes the bot's webhook; a join of
the bot's channel is attributed through the user's `/start`; pixels for TG
events came next (2026-10-15, `docs/pixels.md`), then per-click invite
links, so a join without /start is attributed too, then several channels
per bot and editing a bot (2026-10-15, below).

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
`GET/POST /telegram-bots`, `PATCH /telegram-bots/{id}` (`welcomeText`,
`token` — see "Changing the token"),
`DELETE /telegram-bots/{id}`, and the bot's channels
`POST /telegram-bots/{id}/channels` (`channel`),
`DELETE /telegram-bots/{id}/channels/{channelId}` (see "Channels").
Connecting:

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

## Channels

A bot has up to `MaxChannels` (10) channels (`telegram_channels`,
migration 00078 — the one channel of before moved there). The first can
be given on connect; more are added later, each checked like the first
(`getChat`; the bot an administrator that can invite; the public t.me link
or one `createChatInviteLink` made now) — Telegram's refusal is a
`channel` field error, a channel already on the bot 409, an 11th a field
error. Removing a channel drops it and its invite-link pool: the free links
were never handed out, the claimed ones stay valid in Telegram (someone who
just clicked still gets in) but their joins are no longer attributed. The
channel and its members in Telegram are untouched.

- **/start** is answered with the welcome text and a button per channel
  (oldest first; no welcome text: the first channel's link as the text).
- **Join requests** to any of the bot's channels are approved.
- **TG_JOIN** on a join of any of them, naming the channel
  (`event.TelegramChannelID`, ClickHouse `tg_channel_id`, schema 019).
  Through /start: once per click and channel (`telegram_joins`, migration
  00084) — joining two channels from one click is two joins, a re-sent
  update is none. Through a per-click link: once per link (one click, one
  channel).
- **Editing** a bot is its welcome text, its token (below) and its
  channels.
- Ids in the API's paths and in `/tgj/` that are not ULIDs are 404 before
  Postgres (its `ulid` domain would refuse them as an error).

## Changing the token

`PATCH /telegram-bots/{id}` with `token` (Settings: the key icon on a bot):
for a token revoked and reissued in @BotFather. `getMe` must name the bot
already connected (`tg_bot_id`); another bot's token is a `token` field
error saying to remove this bot and connect the other — switching bots
would orphan the channels, links and /start history, which all belong to
this bot. Then `setWebhook` with the new token and a **new secret**, and
the token (encrypted) and secret hash are stored; the old secret is
refused from then on. Channels, invite links and `telegram_users` stay. If
storing fails after Telegram accepted the webhook, updates are refused
(and re-sent by Telegram) until the change is made again — logged as an
error. Audit: `telegram_bot.token_changed`.

## Joins by channel in reports

- Report builder: the **Telegram channel** dimension (`telegram_channel`,
  `chstore.DimTelegramChannel`) and the metrics **Bot starts** (`tgStarts`,
  TG_START) and **Channel joins** (`tgJoins`, TG_JOIN), counted off
  tracking_events with the PWA installs. By channel, joins split per
  channel; clicks, starts and joins from before migration 00084 / schema
  019 are in "No channel". Conversions cannot be split by channel (one
  click may join several), so revenue/conversion columns are "—" there.
  Both metrics can be used in custom metric formulas and report presets.
- Labels: `GET /telegram-channels` (`analytics.read`) — id, title, bot
  username — so whoever reads reports sees "Title (@bot)", not ids,
  without access to the bots' settings.
- Settings shows each channel's joins over the last 30 days (the same
  report).

Joins marked before migration 00084 moved to `telegram_joins` when their
bot had exactly one channel; with several, which channel was not recorded
and the mark was dropped (such a click joining again counts once more).

## Per-click invite links

To send visitors straight to a channel, use its **channel link per click**
(Settings, shown per channel on a configured deployment) as the
destination: `TELEGRAM_WEBHOOK_BASE_URL/tgj/{channelId}?click={click_id}`.
`/tgj/{botId}` — the form Phase 30.29 showed — still works and means the
bot's first channel.

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
  free links per channel, at most 20 new per channel per run
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
  channel (or a bot without channels): 404. `Cache-Control: no-store`.
- **Join.** `chat_member` carries the link the user joined through
  (`invite_link`, in full for links the bot created). A claimed link of
  that channel: TG_JOIN on its click, once per link (`joined_at`). The link
  already used (passed on to someone else), or not one of ours (the shared
  link): the /start path as before. The link's click not in ClickHouse yet
  and claimed under 10 minutes ago: 503, Telegram re-sends the update;
  later: nothing recorded.
- **Expiry.** Claimed links are revoked (`revokeChatInviteLink`) and
  forgotten after `InviteLinkTTL` (30 days), up to 100 per channel per run; a
  link Telegram refuses to revoke is forgotten too, one it could not be
  asked about is kept for the next run.
- Settings shows each channel's link with a copy button and its free
  links left (`freeInviteLinks`; 0 is shown as a warning).

The click id in `/tgj/` is not signed, like the `/start` payload: anyone can
claim links with made-up ids. That only spends the pool (refilled each
minute; meanwhile the shared link) — a made-up click is not found when its
join comes, so nothing is recorded.

## Not done

Switching a connected bot to another bot (remove and connect the other).

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

## Verified (2026-10-15, token change and joins by channel)

- Tests: `TestChangeToken` (another bot's token, malformed, refused by
  Telegram, another organization — nothing changed; the same bot's new
  token: webhook set again with a new secret, the old one 404, the /start
  made before still attributed), `TestChannelNames` (scoped),
  `TestWebhookSeveralChannels` (a TG_JOIN per channel naming it, re-sent
  join not counted), chstore `TestReportTelegramByChannel` (counts by
  channel, clicks in the empty bucket, conversions not claimed, filter),
  analytics `TestReportTelegramMetrics`. Mutation-checked: per-channel
  dedup, the event's channel, the same-bot check, the new secret.
  Migration 00084 down and up.
- Live on the dev stack with a fake Bot API (`TELEGRAM_API_URL`): connect
  in the UI, a second channel, another bot's token refused at the field,
  the new token accepted (channels kept); Telegram-shaped updates to the
  tracker — old secret 404, /start and joins of both channels on a dev
  click → two TG_JOIN in ClickHouse with their channel ids; Settings
  showed 1 join per channel; the report builder by Telegram channel showed
  "Live A (@flox_live_bot)" / "Live B" with 1 join each and older joins in
  "No channel". A viewer reads `/telegram-channels` (200), not
  `/telegram-bots` (403).

## Verified (2026-10-15, channels and editing)

- Migration 00078 up → down → up on a dev bot with a channel and two pool
  links (one claimed): the channel and links moved and came back; a bot
  without a channel got none.
- Tests (`channels_test.go`): add a public and a private channel after
  connecting, the same one again 409, malformed/unknown channel field
  errors, up to 10 then a field error, welcome edited (trimmed) and too
  long refused, another organization's update/add/remove 404 and list
  empty, remove then 404; with two channels a button each, TG_JOIN on the
  second channel, none for the first from the same click, join request to
  the second approved, a pool per channel, a link reported in the other
  channel not counted. The api's route and reference isolation tests cover
  the new routes (`channels` → a seeded channel; `tgChatId` is Telegram's).
  Mutation-checked: the channel limit, the organization filter of update
  and of channel delete, only the first channel matched, one button.
- Live on the dev stack: `/tgj/{channelId}` of the second channel → its
  pool link (same on reload), `/tgj/{botId}` → the first channel's shared
  link (empty pool), malformed id 404; /start answered with two buttons; a
  join through the second channel's link → TG_JOIN in ClickHouse; API
  malformed ids 404, a bad channel 422. In Chrome: the bot with two
  channels, the welcome edited (toast, text updated, audit
  `telegram_bot.updated`), an invalid channel refused inline, a channel
  removed after confirming (toast, 1/10, audit `telegram_bot.channel_removed`).
  Adding a channel for real was not done: it needs Telegram.

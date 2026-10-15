-- Several channels per Telegram bot (docs/telegram.md "Channels").
--
-- telegram_channels: the channels a bot administers — joining any of them
-- is TG_JOIN (once per click). tg_chat_id is Telegram's id of the channel;
-- invite_link its shared link (the public t.me link, or one the bot made
-- on adding it). The bots' one channel (telegram_bots.channel_*) moves
-- here, and the per-click invite links (00077) are now per channel.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE telegram_channels (
  id               ulid PRIMARY KEY,
  bot_id           ulid NOT NULL REFERENCES telegram_bots (id) ON DELETE CASCADE,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  tg_chat_id       bigint NOT NULL,
  title            text NOT NULL DEFAULT '',
  invite_link      text NOT NULL DEFAULT '',
  created_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (bot_id, tg_chat_id)
);

CREATE INDEX telegram_channels_org_idx ON telegram_channels (organization_id);

INSERT INTO telegram_channels (id, bot_id, organization_id, tg_chat_id, title, invite_link, created_at)
SELECT flox_new_ulid(), id, organization_id, channel_id, channel_title, channel_invite_link, created_at
FROM telegram_bots WHERE channel_id IS NOT NULL;

ALTER TABLE telegram_invite_links ADD COLUMN channel_id ulid REFERENCES telegram_channels (id) ON DELETE CASCADE;
UPDATE telegram_invite_links l SET channel_id = c.id FROM telegram_channels c WHERE c.bot_id = l.bot_id;
DELETE FROM telegram_invite_links WHERE channel_id IS NULL;
ALTER TABLE telegram_invite_links ALTER COLUMN channel_id SET NOT NULL;

DROP INDEX telegram_invite_links_free_idx;
CREATE INDEX telegram_invite_links_free_idx ON telegram_invite_links (channel_id, created_at) WHERE click_id IS NULL;
DROP INDEX telegram_invite_links_click_idx;
CREATE INDEX telegram_invite_links_click_idx ON telegram_invite_links (channel_id, click_id) WHERE click_id IS NOT NULL;

ALTER TABLE telegram_bots DROP COLUMN channel_id, DROP COLUMN channel_title, DROP COLUMN channel_invite_link;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Back to one channel per bot: the first one each bot got.
ALTER TABLE telegram_bots
  ADD COLUMN channel_id bigint,
  ADD COLUMN channel_title text NOT NULL DEFAULT '',
  ADD COLUMN channel_invite_link text NOT NULL DEFAULT '';

UPDATE telegram_bots b SET channel_id = c.tg_chat_id, channel_title = c.title, channel_invite_link = c.invite_link
FROM (SELECT DISTINCT ON (bot_id) * FROM telegram_channels ORDER BY bot_id, created_at, id) c
WHERE c.bot_id = b.id;

DELETE FROM telegram_invite_links l WHERE NOT EXISTS (
  SELECT 1 FROM telegram_bots b JOIN telegram_channels c ON c.bot_id = b.id AND c.tg_chat_id = b.channel_id
  WHERE c.id = l.channel_id);

DROP INDEX telegram_invite_links_free_idx;
CREATE INDEX telegram_invite_links_free_idx ON telegram_invite_links (bot_id, created_at) WHERE click_id IS NULL;
DROP INDEX telegram_invite_links_click_idx;
CREATE INDEX telegram_invite_links_click_idx ON telegram_invite_links (bot_id, click_id) WHERE click_id IS NOT NULL;
ALTER TABLE telegram_invite_links DROP COLUMN channel_id;

DROP TABLE telegram_channels;
-- +goose StatementEnd

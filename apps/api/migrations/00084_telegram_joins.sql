-- TG_JOIN per channel (docs/telegram.md "Channels").
--
-- telegram_joins: a click's joins through /start, one row per channel —
-- so one click joining two of the bot's channels is two TG_JOINs, and a
-- join delivered twice is one. Replaces telegram_users.joined_click_id,
-- which marked one join per click whichever channel.
--
-- Joins marked before this move here when their bot has exactly one
-- channel (then that was the channel); with several, which one is not
-- known and the mark is dropped — such a click joining again counts once
-- more. Per-click invite links keep their own mark (telegram_invite_links.
-- joined_at): a link is of one channel.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE telegram_joins (
  channel_id       ulid NOT NULL REFERENCES telegram_channels (id) ON DELETE CASCADE,
  tg_user_id       bigint NOT NULL,
  click_id         text NOT NULL,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  joined_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (channel_id, tg_user_id, click_id)
);

CREATE INDEX telegram_joins_org_idx ON telegram_joins (organization_id);

INSERT INTO telegram_joins (channel_id, tg_user_id, click_id, organization_id, joined_at)
SELECT c.id, u.tg_user_id, u.joined_click_id, u.organization_id, COALESCE(u.joined_at, now())
FROM telegram_users u
JOIN telegram_channels c ON c.bot_id = u.bot_id
WHERE u.joined_click_id <> ''
  AND (SELECT count(*) FROM telegram_channels c2 WHERE c2.bot_id = u.bot_id) = 1;

ALTER TABLE telegram_users DROP COLUMN joined_click_id, DROP COLUMN joined_at;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE telegram_users
  ADD COLUMN joined_click_id text NOT NULL DEFAULT '',
  ADD COLUMN joined_at timestamptz;

UPDATE telegram_users u SET joined_click_id = j.click_id, joined_at = j.joined_at
FROM (SELECT DISTINCT ON (c.bot_id, j.tg_user_id) c.bot_id, j.tg_user_id, j.click_id, j.joined_at
      FROM telegram_joins j JOIN telegram_channels c ON c.id = j.channel_id
      ORDER BY c.bot_id, j.tg_user_id, j.joined_at DESC) j
WHERE j.bot_id = u.bot_id AND j.tg_user_id = u.tg_user_id AND j.click_id = u.click_id;

DROP TABLE telegram_joins;
-- +goose StatementEnd

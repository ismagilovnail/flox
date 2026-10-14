-- Telegram bots: TG_START and TG_JOIN (§43, docs/telegram.md).
--
-- telegram_bots: an organization's own bot, connected by its token. FLOX
-- sets the bot's webhook to the tracker (/tg/{id}); Telegram signs each
-- call with the webhook secret, of which only the SHA-256 is kept. The
-- token is encrypted with INTEGRATION_ENCRYPTION_KEY (the api alone uses
-- it, to talk to Telegram). tg_bot_id is Telegram's id of the bot: one bot
-- has one webhook, so it belongs to one organization. channel_*: the
-- channel the bot is an admin of (optional) — the welcome message links to
-- channel_invite_link, and joining it is TG_JOIN.
--
-- telegram_users: who started which bot from which click. The /start
-- payload is the click id; a join of the bot's channel is attributed to
-- the click of the same Telegram user's last /start (joined_click_id: the
-- click whose TG_JOIN was recorded, so it is recorded once).

-- +goose Up
-- +goose StatementBegin
CREATE TABLE telegram_bots (
  id                   ulid PRIMARY KEY,
  organization_id      ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  tg_bot_id            bigint NOT NULL UNIQUE,
  username             text NOT NULL,
  token_encrypted      text NOT NULL,
  webhook_secret_hash  text NOT NULL,
  welcome_text         text NOT NULL DEFAULT '',
  channel_id           bigint,
  channel_title        text NOT NULL DEFAULT '',
  channel_invite_link  text NOT NULL DEFAULT '',
  created_by_user_id   ulid REFERENCES users (id) ON DELETE SET NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX telegram_bots_org_idx ON telegram_bots (organization_id);

CREATE TRIGGER telegram_bots_set_updated_at
  BEFORE UPDATE ON telegram_bots
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE telegram_users (
  bot_id           ulid NOT NULL REFERENCES telegram_bots (id) ON DELETE CASCADE,
  tg_user_id       bigint NOT NULL,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  click_id         text NOT NULL,
  started_at       timestamptz NOT NULL DEFAULT now(),
  joined_click_id  text NOT NULL DEFAULT '',
  joined_at        timestamptz,
  PRIMARY KEY (bot_id, tg_user_id)
);

CREATE INDEX telegram_users_org_idx ON telegram_users (organization_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE telegram_users;
DROP TABLE telegram_bots;
-- +goose StatementEnd

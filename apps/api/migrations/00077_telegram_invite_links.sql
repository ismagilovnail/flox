-- Per-click invite links of a bot's channel (docs/telegram.md "Per-click
-- invite links"): a join through one is TG_JOIN on its click, with or
-- without a /start.
--
-- Telegram creates the links (createChatInviteLink), and the redirect may
-- not call it (CLAUDE.md #9), so the worker keeps a pool of them per bot:
-- click_id NULL is a free link. The tracker's /tgj/{botId}?click= claims
-- one for the click (FOR UPDATE SKIP LOCKED) and redirects to it; the same
-- click asking again gets the same link. joined_at: the click's TG_JOIN is
-- recorded (once per link). The worker revokes claimed links after a while
-- and deletes their rows.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE telegram_invite_links (
  invite_link      text PRIMARY KEY,
  bot_id           ulid NOT NULL REFERENCES telegram_bots (id) ON DELETE CASCADE,
  organization_id  ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  created_at       timestamptz NOT NULL DEFAULT now(),
  click_id         text,
  claimed_at       timestamptz,
  joined_at        timestamptz,
  CHECK ((click_id IS NULL) = (claimed_at IS NULL))
);

CREATE INDEX telegram_invite_links_free_idx ON telegram_invite_links (bot_id, created_at) WHERE click_id IS NULL;
CREATE INDEX telegram_invite_links_click_idx ON telegram_invite_links (bot_id, click_id) WHERE click_id IS NOT NULL;
CREATE INDEX telegram_invite_links_claimed_idx ON telegram_invite_links (claimed_at) WHERE click_id IS NOT NULL;
CREATE INDEX telegram_invite_links_org_idx ON telegram_invite_links (organization_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE telegram_invite_links;
-- +goose StatementEnd

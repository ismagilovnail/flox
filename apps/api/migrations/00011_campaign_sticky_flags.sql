-- +goose Up
-- +goose StatementBegin

-- §39-STICKY's three config flags. Phase 17 built §35's literal table list,
-- which never mentioned them; Phase 21 (the tracking engine) is the first
-- code that actually needs to read them, so they land here rather than
-- being guessed at earlier.
--
-- Default false: sticky is opt-in per campaign. A campaign that has never
-- heard of sticky routing behaves exactly as it did before this migration.
ALTER TABLE campaigns
  ADD COLUMN sticky_flow                boolean NOT NULL DEFAULT false,
  -- Reuse the original click_id when a returning visitor is restored to
  -- their sticky flow, so the whole journey stays one attribution chain.
  ADD COLUMN sticky_flow_keep_click_id  boolean NOT NULL DEFAULT false,
  -- true  = keep the saved flow even if it is now inactive
  -- false = drop the cookie and re-pick
  ADD COLUMN sticky_flow_skip_inactive  boolean NOT NULL DEFAULT false;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE campaigns
  DROP COLUMN sticky_flow,
  DROP COLUMN sticky_flow_keep_click_id,
  DROP COLUMN sticky_flow_skip_inactive;
-- +goose StatementEnd

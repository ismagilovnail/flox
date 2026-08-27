-- +goose Up
-- +goose StatementBegin

-- §54/Phase 30: incoming-postback webhook authentication. Before this,
-- GET/POST /postback/{networkId} (apps/tracker/postback.go) trusted the
-- URL path's {networkId} alone — a syntactically valid, existing ULID
-- was the only check. Any client that observed or guessed that ULID
-- (via a leaked URL, a referrer header, browser history, a log line)
-- could POST arbitrary conversion data as that network. postback_secret_hash
-- is a per-network random token FLOX embeds in the incoming postback URL
-- it hands the network operator (?secret=...); apps/tracker hashes an
-- incoming request's secret and compares it against this column before
-- ever recording a conversion.
--
-- Hashed, not stored plaintext — same one-way-hash-at-rest precedent as
-- sessions.token_hash/api_keys.key_hash (migrations 00010, 00020): the
-- raw secret is shown once at creation (or after an explicit
-- regenerate), never persisted in recoverable form. DEFAULT '' matches
-- users.password_hash's own sentinel-for-"not set yet" pattern
-- (migration 00020) — SHA-256("") can never equal a real request's
-- hashed secret, so an empty hash safely rejects every postback rather
-- than needing a separate NULL check at the comparison site.
ALTER TABLE networks ADD COLUMN postback_secret_hash text NOT NULL DEFAULT '';

-- Partial (excludes the '' sentinel, same reasoning as memberships.
-- invite_token_hash's own partial unique index, migration 00020): a real
-- collision here would let one network authenticate incoming postbacks
-- as another, so this is cheap, worthwhile insurance against an RNG bug
-- even though 24 random bytes make an actual collision astronomically
-- unlikely.
CREATE UNIQUE INDEX networks_postback_secret_hash_key ON networks (postback_secret_hash) WHERE postback_secret_hash != '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX networks_postback_secret_hash_key;
ALTER TABLE networks DROP COLUMN postback_secret_hash;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin

-- Email verification, gating login until a signup proves it owns the
-- email it registered with — the requirement migration 00020's own signup
-- flow deliberately skipped (that phase had no SMTP/email-provider
-- integration anywhere; infra/docker-compose.dev.yml's mailhog dev
-- catcher and a real SMTP client both landed later, staged exactly for
-- this). NULL = not verified, matching password_hash's own "empty/absent
-- sentinel, no separate nullability check needed at read sites" style
-- immediately above it.
ALTER TABLE users ADD COLUMN email_verified_at timestamptz;

-- Every user that exists before this migration runs (real dev/test data,
-- e2e fixtures, anyone already using this system) is grandfathered in as
-- verified — this migration gates NEW signups, it must not silently lock
-- existing accounts out of their own workspace.
UPDATE users SET email_verified_at = now() WHERE email_verified_at IS NULL;

-- Token lives on the users row itself, not a separate table — same
-- reasoning as memberships.invite_token_hash right above this migration's
-- own precedent (00020): a verified user IS this same row transitioning,
-- not a new entity. One-way-hash-at-rest, same as sessions.token_hash and
-- memberships.invite_token_hash.
ALTER TABLE users ADD COLUMN email_verification_token_hash text;
ALTER TABLE users ADD COLUMN email_verification_token_expires_at timestamptz;

-- Partial unique index, same reasoning as
-- memberships_invite_token_hash_key: only guards against two pending
-- verification tokens colliding, not about NULL semantics.
CREATE UNIQUE INDEX users_email_verification_token_hash_key ON users (email_verification_token_hash) WHERE email_verification_token_hash IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX users_email_verification_token_hash_key;
ALTER TABLE users DROP COLUMN email_verification_token_expires_at;
ALTER TABLE users DROP COLUMN email_verification_token_hash;
ALTER TABLE users DROP COLUMN email_verified_at;
-- +goose StatementEnd

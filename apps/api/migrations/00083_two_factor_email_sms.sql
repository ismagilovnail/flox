-- Two-factor authentication by email and SMS codes (docs/auth.md
-- "Second factor: email and SMS"), next to the TOTP app (00032).
--
-- users.two_factor_method is the user's one second factor: 'totp' (the
-- app, totp_secret), 'email' (a code to the account's email) or 'sms' (a
-- code to users.phone). NULL: none. Existing confirmed TOTP users become
-- 'totp'. phone is the confirmed number (E.164); phone_pending the one
-- being confirmed.
--
-- two_factor_codes: one-time codes sent by email/SMS — only their SHA-256
-- is kept. purpose 'login' belongs to a pending login (deleted with it);
-- 'enroll' confirms a method being set up and carries the backup codes
-- (hashes) that become the user's once it is confirmed. attempts counts
-- wrong guesses; used_at marks the one success.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE users
  ADD COLUMN two_factor_method text CHECK (two_factor_method IN ('totp', 'email', 'sms')),
  ADD COLUMN phone text NOT NULL DEFAULT '',
  ADD COLUMN phone_pending text NOT NULL DEFAULT '';

UPDATE users SET two_factor_method = 'totp' WHERE totp_enabled_at IS NOT NULL;

CREATE TABLE two_factor_codes (
  id                ulid PRIMARY KEY,
  user_id           ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  purpose           text NOT NULL CHECK (purpose IN ('login', 'enroll')),
  method            text NOT NULL CHECK (method IN ('email', 'sms')),
  pending_login_id  ulid REFERENCES pending_totp_logins (id) ON DELETE CASCADE,
  target            text NOT NULL,
  code_hash         text NOT NULL,
  backup_code_hashes text[] NOT NULL DEFAULT '{}',
  attempts          integer NOT NULL DEFAULT 0,
  expires_at        timestamptz NOT NULL,
  used_at           timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  CHECK ((purpose = 'login') = (pending_login_id IS NOT NULL))
);

CREATE INDEX two_factor_codes_user_created_idx ON two_factor_codes (user_id, created_at DESC);
CREATE INDEX two_factor_codes_pending_idx ON two_factor_codes (pending_login_id) WHERE pending_login_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE two_factor_codes;
ALTER TABLE users DROP COLUMN two_factor_method, DROP COLUMN phone, DROP COLUMN phone_pending;
-- +goose StatementEnd

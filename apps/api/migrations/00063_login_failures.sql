-- Tech audit 2026-10-06, API2: failed sign-ins per account, in Postgres so
-- the limit holds while Redis (the per-IP limiter) is down.
--
-- Keyed by sha256(lower(email)), not user_id: an email with no account
-- counts exactly like one with an account, so a lockout never tells an
-- attacker which emails exist. Hashed so attacker-typed addresses are not
-- kept in the clear. internal/auth owns it; internal/retention deletes
-- rows whose window is over. Not tenant data — a sign-in precedes any
-- organization.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE login_failures (
  email_hash    text PRIMARY KEY,
  failures      integer NOT NULL,
  window_start  timestamptz NOT NULL
);

CREATE INDEX login_failures_window_start_idx ON login_failures (window_start);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE login_failures;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin

-- Security settings (§30, Settings' own deferred "Security" sub-panel) —
-- real backend for the three controls the mock UI already committed to:
-- an org-wide "require 2FA" policy, a configurable session timeout, and
-- an IP allowlist. Zero of this is specified anywhere in FLOX-master-
-- prompt-v3.md (confirmed by grep before writing this migration) — the
-- mock's own shape (features/settings/security-panel.tsx,
-- lib/mock/settings.ts's SecuritySettings) is the only existing design,
-- so this migration and the Go package built against it are new product
-- design, not a spec transcription.
--
-- session_timeout_minutes/ip_allowlist/two_factor_required live directly
-- on `organizations`, same as timezone/currency — this is the same
-- object Organization settings already reads/writes, just a different
-- UI panel and a different Go handler (mirroring how Organization and
-- API Keys are already two separate handlers over related tables).
-- Defaults preserve today's real, already-live behavior for every
-- existing org: 43200 minutes (30 days) matches auth.sessionTTL's
-- former hardcoded constant exactly (this migration is what makes it
-- finally configurable, not what changes anyone's behavior on day one)
-- — the CHECK's own upper bound must cover that default (60 days'
-- headroom above it, not just the frontend panel's own tighter
-- 15-20160-minute/14-day dropdown range, which is a UI convenience, not
-- the real ceiling); an empty ip_allowlist means "no restriction," the
-- only sane default for a feature nobody has configured yet;
-- two_factor_required defaults off, since forcing enrollment on every
-- existing member the moment this ships would lock out anyone who
-- hasn't set up TOTP yet.
ALTER TABLE organizations
  ADD COLUMN session_timeout_minutes integer NOT NULL DEFAULT 43200
    CHECK (session_timeout_minutes BETWEEN 15 AND 86400),
  ADD COLUMN ip_allowlist text[] NOT NULL DEFAULT '{}',
  ADD COLUMN two_factor_required boolean NOT NULL DEFAULT false;

-- Real per-user TOTP. totp_secret is the only symmetrically-ENCRYPTED
-- (not hashed) secret anywhere in this codebase — it must be
-- decryptable to check a live 6-digit code against it, unlike a
-- password or session token, which only ever need one-way comparison.
-- Encrypted at the application layer (apps/internal/totp.EncryptSecret,
-- AES-256-GCM, key from TOTP_ENCRYPTION_KEY) before this column ever
-- sees it — Postgres itself stores only ciphertext.
-- totp_enabled_at NULL means "no confirmed secret" — covers both "never
-- started enrollment" and "generated a secret but never verified the
-- first code" identically; Service.EnrollTOTP overwrites a pending
-- (unconfirmed) secret freely on retry, exactly like a draft.
ALTER TABLE users
  ADD COLUMN totp_secret text,
  ADD COLUMN totp_enabled_at timestamptz;

-- One-time recovery codes, same one-way-hash-at-rest precedent as
-- sessions.token_hash/api_keys.key_hash — the plaintext codes are shown
-- to the user exactly once, at enrollment, never again.
CREATE TABLE user_totp_backup_codes (
  id         ulid PRIMARY KEY,
  user_id    ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  code_hash  text NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX user_totp_backup_codes_user_id_idx ON user_totp_backup_codes (user_id);

-- A short-lived, single-use bridge between "password verified" and "a
-- real session exists" while a second factor is still outstanding —
-- deliberately a SEPARATE table from `sessions`, not a flag column on
-- it: tenant.Service.ResolveSession's own query only ever looks at
-- `sessions`, so a half-authenticated row here can never be mistaken
-- for a real, tenant-scoped session no matter what future code touches
-- either table. requires_enrollment distinguishes "this user already
-- has TOTP, just needs to enter a code" from "the org now requires 2FA
-- and this user must set it up before continuing" — both cases hand the
-- frontend the same opaque token, just a different next screen.
CREATE TABLE pending_totp_logins (
  id                   ulid PRIMARY KEY,
  user_id              ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  organization_id      ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  token_hash           text NOT NULL UNIQUE,
  requires_enrollment  boolean NOT NULL DEFAULT false,
  created_at           timestamptz NOT NULL DEFAULT now(),
  expires_at           timestamptz NOT NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE pending_totp_logins;
DROP TABLE user_totp_backup_codes;
ALTER TABLE users DROP COLUMN totp_secret, DROP COLUMN totp_enabled_at;
ALTER TABLE organizations
  DROP COLUMN session_timeout_minutes,
  DROP COLUMN ip_allowlist,
  DROP COLUMN two_factor_required;
-- +goose StatementEnd

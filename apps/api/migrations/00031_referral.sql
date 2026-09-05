-- +goose Up
-- +goose StatementBegin

-- Referral Program (§30.7, Phase 14.8) — real backend. Per the frontend
-- mock's own design note (lib/mock/referral.ts, now superseded by lib/
-- referral.ts): one referral account per WORKSPACE (org-refers-org, team-
-- scoped per §36-TENANCY), not per individual member — the code/link and
-- balance belong to the tenant, like everything else in the app. Amounts
-- are flat USD (FLOX's own commission to the tenant, not tenant traffic
-- revenue), so §50-FX's original-currency/event-date normalization
-- deliberately does not apply here — same reasoning the mock's own doc
-- comment already gave.
--
-- Four tables:
--   referral_codes       — one row per org, the code it hands out to
--                           others. Lazily created on first real use
--                           (apps/internal/referral.Repository.
--                           EnsureCode), not seeded here.
--   referral_signups     — a real, automatically-created row every time
--                           a NEW org signs up using another org's real
--                           code (wired through apps/internal/auth.
--                           Service.Signup via a small ReferralRecorder
--                           interface, so auth never imports this
--                           package directly). referred_organization_id
--                           is UNIQUE — an org can only ever be "the
--                           referred org" once, matching the fact it
--                           only ever signs up once.
--   referral_payouts     — the pending -> approved -> paid|rejected
--                           state machine §30.7 asks for.
--   referral_transactions — the append-only ledger the spec calls for
--                           ("balance history is consistent and
--                           immutable except through audited
--                           transactions"): no UPDATE/DELETE path exists
--                           anywhere in apps/internal/referral on
--                           purpose. The balance itself is NEVER stored
--                           — always computed by aggregating this table
--                           plus referral_payouts, same design the mock
--                           already committed to.
--
-- No commission formula is specified anywhere in the spec (no rate, no
-- tie to referred-org spend) — same "manual entry first" precedent
-- CLAUDE.md's own §27-COST rule already established for ad spend cost.
-- A signup is attributed to its referrer for real and automatically;
-- crediting an actual dollar amount for it is a deliberate, explicit,
-- audited Owner/Admin action (an `accrual` transaction, optionally tied
-- to a specific referral_signups row via related_signup_id) — not an
-- invented automatic formula this spec never defined.
CREATE TABLE referral_codes (
  organization_id ulid PRIMARY KEY REFERENCES organizations (id) ON DELETE CASCADE,
  code            text NOT NULL UNIQUE,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE referral_signups (
  id                        ulid PRIMARY KEY,
  organization_id           ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  referred_organization_id  ulid NOT NULL UNIQUE REFERENCES organizations (id) ON DELETE CASCADE,
  name                      text NOT NULL,
  email                     text NOT NULL,
  status                    text NOT NULL DEFAULT 'trial' CHECK (status IN ('trial', 'active_customer', 'churned')),
  signed_up_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX referral_signups_organization_id_idx ON referral_signups (organization_id);

CREATE TABLE referral_payouts (
  id                    ulid PRIMARY KEY,
  organization_id       ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  amount                numeric(12, 2) NOT NULL CHECK (amount > 0),
  currency              text NOT NULL DEFAULT 'USD' CHECK (currency = 'USD'),
  status                text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'paid', 'rejected')),
  requested_at          timestamptz NOT NULL DEFAULT now(),
  requested_by_user_id  ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  resolved_at           timestamptz,
  resolved_by_user_id   ulid REFERENCES users (id) ON DELETE SET NULL,
  note                  text NOT NULL DEFAULT ''
);

CREATE INDEX referral_payouts_organization_id_idx ON referral_payouts (organization_id);

-- amount's sign carries meaning (matches the frontend mock's own
-- convention): positive for accrual/positive adjustment, negative for a
-- negative adjustment or a payout_paid row (stored as -payout.amount).
CREATE TABLE referral_transactions (
  id                  ulid PRIMARY KEY,
  organization_id     ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  type                text NOT NULL CHECK (type IN ('accrual', 'adjustment', 'payout_paid')),
  amount              numeric(12, 2) NOT NULL CHECK (amount <> 0),
  currency            text NOT NULL DEFAULT 'USD' CHECK (currency = 'USD'),
  description         text NOT NULL,
  related_signup_id   ulid REFERENCES referral_signups (id) ON DELETE SET NULL,
  related_payout_id   ulid REFERENCES referral_payouts (id) ON DELETE SET NULL,
  created_by_user_id  ulid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX referral_transactions_organization_id_idx ON referral_transactions (organization_id);

-- Read is granted to all six roles (anyone can view the dashboard and
-- request a payout — matches the mock's own "visible to everyone, not
-- gated by canManage" rule for Request Payout); write is Owner/Admin
-- ONLY, deliberately narrower than Custom Metrics'/Report Presets' own
-- Owner/Admin/Manager tiering — the mock's real gating check
-- (`isOwnerOrAdmin`, referral-dashboard.tsx/use-current-member.ts) never
-- included Manager, and money movement warrants the narrower tier.
INSERT INTO permissions (id, key, description) VALUES
  ('7X03F7ESHP2AX6VADE63D1J9FY', 'referral.read', 'View the referral dashboard and request a payout.'),
  ('1ZY8M6C9G3GSES4H1CDBNA5XQ8', 'referral.write', 'Approve/reject/pay referral payouts and record referral ledger entries.');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = ANY (ARRAY['referral.read', 'referral.write'])
WHERE r.key IN ('Owner', 'Admin');

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r JOIN permissions p ON p.key = 'referral.read'
WHERE r.key IN ('Manager', 'Buyer', 'Analyst', 'Viewer');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT id FROM permissions WHERE key IN ('referral.read', 'referral.write')
);
DELETE FROM permissions WHERE key IN ('referral.read', 'referral.write');
DROP TABLE referral_transactions;
DROP TABLE referral_payouts;
DROP TABLE referral_signups;
DROP TABLE referral_codes;
-- +goose StatementEnd

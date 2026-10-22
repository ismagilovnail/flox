-- +goose Up
-- +goose StatementBegin

-- Plans paid online through YooKassa (audit BIZ-1 part 2; docs/billing.md
-- "Payments"). Prepaid periods, no automatic charges.

-- The plan's price in kopecks; NULL: not sold online (Free, custom deals).
-- Draft numbers, like the USD prices — BIZ-2 settles them.
ALTER TABLE plans ADD COLUMN price_rub bigint CHECK (price_rub > 0);
UPDATE plans SET price_rub = 490000 WHERE code = 'start';
UPDATE plans SET price_rub = 1490000 WHERE code = 'pro';
UPDATE plans SET price_rub = 3990000 WHERE code = 'business';

-- paid_until: the end of the paid period; NULL is open-ended (Free, or a
-- plan assigned with apps/cmd/billing). The plan stays on for a grace
-- period after it, then apps/worker moves the organization to Free.
-- reminded_at: the "renew" email for this paid_until was sent.
ALTER TABLE organization_plans
  ADD COLUMN paid_until  timestamptz,
  ADD COLUMN reminded_at timestamptz;

CREATE INDEX organization_plans_paid_until_idx ON organization_plans (paid_until) WHERE ended_at IS NULL AND paid_until IS NOT NULL;

-- One row per checkout. The provider's payment is the truth: a row only
-- changes after its status was read back from the provider, never from a
-- notification's body. applied_at: the payment extended the plan — set
-- once, in the same transaction, so a repeated notification changes
-- nothing.
CREATE TABLE billing_payments (
  id                  ulid PRIMARY KEY,
  organization_id     ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  plan_code           text NOT NULL REFERENCES plans (code),
  months              integer NOT NULL CHECK (months IN (1, 3, 12)),
  amount              bigint NOT NULL CHECK (amount > 0), -- minor units
  currency            text NOT NULL DEFAULT 'RUB',
  provider            text NOT NULL DEFAULT 'yookassa',
  provider_payment_id text,
  confirmation_url    text NOT NULL DEFAULT '',
  status              text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'canceled')),
  cancellation_reason text NOT NULL DEFAULT '',
  created_by_user_id  ulid REFERENCES users (id) ON DELETE SET NULL,
  applied_at          timestamptz,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX billing_payments_provider_key ON billing_payments (provider, provider_payment_id);
CREATE INDEX billing_payments_organization_id_idx ON billing_payments (organization_id, created_at DESC);
CREATE INDEX billing_payments_pending_idx ON billing_payments (created_at) WHERE status = 'pending';

CREATE TRIGGER billing_payments_set_updated_at
  BEFORE UPDATE ON billing_payments
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE billing_payments;
DROP INDEX organization_plans_paid_until_idx;
ALTER TABLE organization_plans DROP COLUMN reminded_at, DROP COLUMN paid_until;
ALTER TABLE plans DROP COLUMN price_rub;
-- +goose StatementEnd

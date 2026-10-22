-- +goose Up
-- +goose StatementBegin

-- Plans and which one each organization is on (audit 2026-10-15, BIZ-1;
-- docs/billing.md). Plans are rows, not code, so a price or a limit
-- changes without a deploy. A NULL limit means unlimited.
--
-- Clicks are SOURCE_CLICK + SOURCE_FILTER per UTC calendar month: filtered
-- and bot traffic costs us the same to take in. Limits are soft — the
-- tracker never stops redirecting over them (internal/billing).
CREATE TABLE plans (
  code           text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9_]{1,31}$'),
  name           text NOT NULL,
  monthly_clicks bigint CHECK (monthly_clicks > 0),
  max_campaigns  integer CHECK (max_campaigns > 0),
  max_domains    integer CHECK (max_domains > 0),
  max_members    integer CHECK (max_members > 0),
  price_cents    integer NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  currency       text NOT NULL DEFAULT 'USD',
  -- false: assignable by hand (a custom deal), not listed on the plans page.
  public         boolean NOT NULL DEFAULT true,
  sort_order     integer NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER plans_set_updated_at
  BEFORE UPDATE ON plans
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Draft numbers: the click tiers follow the competitors' entry plans,
-- prices are placeholders until BIZ-2 measures what a client costs.
INSERT INTO plans (code, name, monthly_clicks, max_campaigns, max_domains, max_members, price_cents, sort_order) VALUES
  ('free',     'Free',     100000,   5,    1,    2,    0,     10),
  ('start',    'Start',    1000000,  50,   5,    5,    4900,  20),
  ('pro',      'Pro',      5000000,  NULL, 20,   15,   14900, 30),
  ('business', 'Business', 20000000, NULL, NULL, NULL, 39900, 40);

-- One row per plan an organization has been on; the open one (ended_at
-- NULL) is the current plan. No row at all means Free since the
-- organization was created — signup writes nothing here.
CREATE TABLE organization_plans (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  plan_code       text NOT NULL REFERENCES plans (code),
  started_at      timestamptz NOT NULL DEFAULT now(),
  ended_at        timestamptz,
  note            text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE UNIQUE INDEX organization_plans_current_key ON organization_plans (organization_id) WHERE ended_at IS NULL;
CREATE INDEX organization_plans_organization_id_idx ON organization_plans (organization_id, started_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE organization_plans;
DROP TABLE plans;
-- +goose StatementEnd

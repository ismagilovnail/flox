-- Online stores reporting their orders (audit UNI-4 part 3;
-- docs/shop-connections.md). An own-site conversion source connects its
-- store; the store's order webhooks reach apps/tracker, which checks their
-- signature and records purchases, repeat purchases and refunds.
--
-- shop_connections: one per conversion source and platform ('shopify'
-- today; WooCommerce later):
--   shop_domain — the store's myshopify.com domain: a webhook from another
--     store is refused even when signed with the same key;
--   secret — the key the store signs webhooks with, AES-256-GCM ciphertext
--     under SHOP_WEBHOOK_ENCRYPTION_KEY (the api encrypts, the tracker
--     decrypts to check a signature); secret_preview its last 4 characters.

-- +goose Up
CREATE TABLE shop_connections (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  network_id      ulid NOT NULL REFERENCES networks (id) ON DELETE CASCADE,
  platform        text NOT NULL CHECK (platform IN ('shopify')),
  shop_domain     text NOT NULL,
  secret          text NOT NULL,
  secret_preview  text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (network_id, platform)
);

CREATE INDEX shop_connections_organization_id_idx ON shop_connections (organization_id);

CREATE TRIGGER shop_connections_set_updated_at
  BEFORE UPDATE ON shop_connections
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE shop_connections;

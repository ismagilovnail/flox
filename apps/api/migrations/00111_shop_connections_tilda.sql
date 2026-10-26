-- Tilda sites (audit UNI-4 part 5; docs/shop-connections.md). Same
-- shop_connections shape: shop_domain is the owner's note of the site (a
-- Tilda webhook does not say which site sent it; may be empty), secret the
-- API key set on the site's Webhook receiver.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE shop_connections DROP CONSTRAINT shop_connections_platform_check;
ALTER TABLE shop_connections ADD CONSTRAINT shop_connections_platform_check
  CHECK (platform IN ('shopify', 'woocommerce', 'tilda'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM shop_connections WHERE platform = 'tilda';
ALTER TABLE shop_connections DROP CONSTRAINT shop_connections_platform_check;
ALTER TABLE shop_connections ADD CONSTRAINT shop_connections_platform_check
  CHECK (platform IN ('shopify', 'woocommerce'));
-- +goose StatementEnd

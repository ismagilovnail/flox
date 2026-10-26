-- WooCommerce stores (audit UNI-4 part 4; docs/shop-connections.md). The
-- shop_connections shape is the same: shop_domain is the store's site
-- address (home_url without the scheme, a path kept), secret the one set
-- on its webhooks.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE shop_connections DROP CONSTRAINT shop_connections_platform_check;
ALTER TABLE shop_connections ADD CONSTRAINT shop_connections_platform_check
  CHECK (platform IN ('shopify', 'woocommerce'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM shop_connections WHERE platform = 'woocommerce';
ALTER TABLE shop_connections DROP CONSTRAINT shop_connections_platform_check;
ALTER TABLE shop_connections ADD CONSTRAINT shop_connections_platform_check
  CHECK (platform IN ('shopify'));
-- +goose StatementEnd

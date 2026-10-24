-- VK Ads as a cost integration (audit UNI-3 part 2;
-- docs/ad-account-connections.md "VK Ads"). Its connection keeps the API
-- client's id (client_id) and secret (client_secret, encrypted), and for
-- an agency the client's username (ad_account_id, '' for the API client's
-- own account). access_token / refresh_token hold the token pair the sync
-- minted, kept and renewed (VK allows 5 tokens at once), encrypted like
-- the rest under ADACCOUNT_ENCRYPTION_KEY.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE traffic_sources DROP CONSTRAINT traffic_sources_cost_integration_check;
ALTER TABLE traffic_sources ADD CONSTRAINT traffic_sources_cost_integration_check
  CHECK (cost_integration IN ('none', 'manual', 'facebook_ads', 'tiktok_ads', 'google_ads', 'yandex_direct', 'vk_ads'));

ALTER TABLE cost_entries DROP CONSTRAINT cost_entries_source_check;
ALTER TABLE cost_entries ADD CONSTRAINT cost_entries_source_check
  CHECK (source IN ('manual', 'facebook_ads', 'tiktok_ads', 'google_ads', 'yandex_direct', 'vk_ads'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE cost_entries DROP CONSTRAINT cost_entries_source_check;
ALTER TABLE cost_entries ADD CONSTRAINT cost_entries_source_check
  CHECK (source IN ('manual', 'facebook_ads', 'tiktok_ads', 'google_ads', 'yandex_direct'));

ALTER TABLE traffic_sources DROP CONSTRAINT traffic_sources_cost_integration_check;
ALTER TABLE traffic_sources ADD CONSTRAINT traffic_sources_cost_integration_check
  CHECK (cost_integration IN ('none', 'manual', 'facebook_ads', 'tiktok_ads', 'google_ads', 'yandex_direct'));
-- +goose StatementEnd

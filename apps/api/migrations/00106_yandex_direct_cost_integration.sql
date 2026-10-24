-- Yandex Direct as a cost integration (audit UNI-3 part 1;
-- docs/ad-account-connections.md "Yandex Direct"). Its connection is the
-- facebook_ads/tiktok_ads shape: ad_account_id holds the advertiser's
-- Direct login (sent as Client-Login), access_token an OAuth token
-- (encrypted like the others, ADACCOUNT_ENCRYPTION_KEY).

-- +goose Up
-- +goose StatementBegin
ALTER TABLE traffic_sources DROP CONSTRAINT traffic_sources_cost_integration_check;
ALTER TABLE traffic_sources ADD CONSTRAINT traffic_sources_cost_integration_check
  CHECK (cost_integration IN ('none', 'manual', 'facebook_ads', 'tiktok_ads', 'google_ads', 'yandex_direct'));

ALTER TABLE cost_entries DROP CONSTRAINT cost_entries_source_check;
ALTER TABLE cost_entries ADD CONSTRAINT cost_entries_source_check
  CHECK (source IN ('manual', 'facebook_ads', 'tiktok_ads', 'google_ads', 'yandex_direct'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE cost_entries DROP CONSTRAINT cost_entries_source_check;
ALTER TABLE cost_entries ADD CONSTRAINT cost_entries_source_check
  CHECK (source IN ('manual', 'facebook_ads', 'tiktok_ads', 'google_ads'));

ALTER TABLE traffic_sources DROP CONSTRAINT traffic_sources_cost_integration_check;
ALTER TABLE traffic_sources ADD CONSTRAINT traffic_sources_cost_integration_check
  CHECK (cost_integration IN ('none', 'manual', 'facebook_ads', 'tiktok_ads', 'google_ads'));
-- +goose StatementEnd

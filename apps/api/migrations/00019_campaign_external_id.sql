-- +goose Up
-- +goose StatementBegin

-- §74/§27-COST, ad-spend import Phase B: cost_entries requires a
-- campaign_id (00009, NOT NULL) but Facebook's/TikTok's spend APIs
-- report by their OWN campaign id, broken down at that platform's own
-- campaign level — and a traffic source (where an ad_account_connection,
-- migration 00018, lives) can fund more than one FLOX campaign
-- (campaigns.traffic_source_id is many-to-one, not one-to-one). Without
-- this column there is no way to know which FLOX campaign a given day's
-- ad-platform-campaign spend belongs to.
--
-- Nullable, no uniqueness constraint: most campaigns will never set
-- this (only ones actually funded by a connected Facebook/TikTok ad
-- account need to), and nothing stops the same external id being
-- reused if an operator genuinely wants two FLOX campaigns to share one
-- ad-platform campaign's spend (split-testing two FLOX setups against
-- one real ad campaign is a legitimate, if unusual, setup) — enforcing
-- uniqueness here would forbid that for no real benefit, since a sync's
-- own upsert-by-(campaign_id, entry_date) key already prevents any
-- double-counting artifact from it.
ALTER TABLE campaigns ADD COLUMN external_campaign_id text NOT NULL DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE campaigns DROP COLUMN external_campaign_id;
-- +goose StatementEnd

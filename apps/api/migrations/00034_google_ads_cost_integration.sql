-- +goose Up
-- +goose StatementBegin

-- §74/§27-COST: Google Ads as a third real adaccount.CostProvider,
-- matching Facebook/TikTok Ads' own per-traffic-source design exactly
-- (trafficsource.CostIntegration's singular "this source uses at most
-- one integration" shape) — not a new org-level integration (see
-- docs/settings-integrations.md's own NEXT section for why Google Ads
-- deliberately did NOT get a Settings > Integrations connect flow).
--
-- Google Ads' real API needs more than the ad_account_id + access_token
-- shape facebook_ads/tiktok_ads use: a developer token, an OAuth client
-- id/secret, and a refresh token (Google Ads access tokens expire in
-- ~1 hour — a long-lived credential here must be a refresh token, minted
-- once by the operator via Google's own OAuth playground since no
-- registered public callback URL exists in this environment, same
-- reasoning apps/internal/adaccount's own package doc already gives for
-- manual-paste over a real OAuth consent flow).
--
-- access_token becomes nullable (google_ads rows never set it — the
-- adapter mints a short-lived one per sync from refresh_token instead).
-- developer_token/client_secret/refresh_token are AES-256-GCM encrypted
-- at rest via a new, dedicated ADACCOUNT_ENCRYPTION_KEY (apps/internal/
-- adaccount's own crypto.go) — the real encryption infrastructure
-- apps/internal/totp established, not a repeat of this table's own
-- original access_token-in-plain-text gap (that column predates any
-- encryption infra in this codebase and is left as-is; retrofitting it
-- is separate, real hardening work, not this migration's job).
-- client_id is stored in plain text — it's a public OAuth client
-- identifier, not a secret, same convention OAuth 2.0 itself uses for
-- the id/secret pair.
ALTER TABLE ad_account_connections ALTER COLUMN access_token DROP NOT NULL;
ALTER TABLE ad_account_connections ADD COLUMN developer_token text;
ALTER TABLE ad_account_connections ADD COLUMN client_id text;
ALTER TABLE ad_account_connections ADD COLUMN client_secret text;
ALTER TABLE ad_account_connections ADD COLUMN refresh_token text;

ALTER TABLE traffic_sources DROP CONSTRAINT traffic_sources_cost_integration_check;
ALTER TABLE traffic_sources ADD CONSTRAINT traffic_sources_cost_integration_check
  CHECK (cost_integration IN ('none', 'manual', 'facebook_ads', 'tiktok_ads', 'google_ads'));

ALTER TABLE cost_entries DROP CONSTRAINT cost_entries_source_check;
ALTER TABLE cost_entries ADD CONSTRAINT cost_entries_source_check
  CHECK (source IN ('manual', 'facebook_ads', 'tiktok_ads', 'google_ads'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE cost_entries DROP CONSTRAINT cost_entries_source_check;
ALTER TABLE cost_entries ADD CONSTRAINT cost_entries_source_check
  CHECK (source IN ('manual', 'facebook_ads', 'tiktok_ads'));

ALTER TABLE traffic_sources DROP CONSTRAINT traffic_sources_cost_integration_check;
ALTER TABLE traffic_sources ADD CONSTRAINT traffic_sources_cost_integration_check
  CHECK (cost_integration IN ('none', 'manual', 'facebook_ads', 'tiktok_ads'));

ALTER TABLE ad_account_connections DROP COLUMN refresh_token;
ALTER TABLE ad_account_connections DROP COLUMN client_secret;
ALTER TABLE ad_account_connections DROP COLUMN client_id;
ALTER TABLE ad_account_connections DROP COLUMN developer_token;
ALTER TABLE ad_account_connections ALTER COLUMN access_token SET NOT NULL;
-- +goose StatementEnd

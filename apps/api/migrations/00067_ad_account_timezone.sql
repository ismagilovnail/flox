-- Tech audit 2026-10-06, AN4: ad platforms report daily spend by calendar
-- day in the ad account's own time zone (Facebook's account timezone_name,
-- TikTok's advertiser timezone, Google Ads' customer.time_zone). Synced
-- cost_entries carry that day as entry_date, while reports cut clicks by
-- the report's zone — a day of spend and a day of clicks can be hours
-- apart. The account's zone is kept here (IANA name, '' until a sync
-- learns it) so a report can say when the two differ.

-- +goose Up
-- +goose StatementBegin
ALTER TABLE ad_account_connections ADD COLUMN timezone text NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE ad_account_connections DROP COLUMN timezone;
-- +goose StatementEnd

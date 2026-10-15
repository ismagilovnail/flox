-- The URL of a flow's PWA stage for the types that are not served by FLOX
-- (docs/pwa-serving.md "PWA types"): pwa_type 'external' — the external
-- PWA's address, with macros ({click_id}, {pwa_link}…); 'ios_app' — the
-- app link iPhone/iPad visitors are sent to (pwa_id, optional, is then the
-- PWA other devices get). '' for 'internal'.

-- +goose Up
ALTER TABLE flows ADD COLUMN pwa_url text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE flows DROP COLUMN pwa_url;

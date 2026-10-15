-- Twitter/X pixels are dropped as a provider (owner's decision, 2026-10-15):
-- they could never send (X's Conversions API needs OAuth 1.0a app
-- credentials FLOX does not have), so any left are removed with their
-- stream-set links and (empty) delivery history, and the provider is no
-- longer allowed.

-- +goose Up
-- +goose StatementBegin
DELETE FROM pixels WHERE provider = 'twitter';
ALTER TABLE pixels DROP CONSTRAINT pixels_provider_check;
ALTER TABLE pixels ADD CONSTRAINT pixels_provider_check CHECK (provider IN ('facebook', 'tiktok', 'snapchat', 'generic'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The provider is allowed again; deleted twitter pixels are not restored.
ALTER TABLE pixels DROP CONSTRAINT pixels_provider_check;
ALTER TABLE pixels ADD CONSTRAINT pixels_provider_check CHECK (provider IN ('facebook', 'tiktok', 'snapchat', 'twitter', 'generic'));
-- +goose StatementEnd

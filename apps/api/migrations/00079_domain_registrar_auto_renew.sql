-- Whether the registrar renews the domain by itself (docs/domains-tracking-links.md,
-- "Cloudflare Registrar"). Read on Verify from a registrar that reports it
-- (Cloudflare Registrar, which also cannot renew through its API — so this is
-- what tells the operator whether the domain lapses). NULL: not known (the
-- registrar does not say, or the domain was never verified).

-- +goose Up
ALTER TABLE domains ADD COLUMN registrar_auto_renew boolean;

-- +goose Down
ALTER TABLE domains DROP COLUMN registrar_auto_renew;

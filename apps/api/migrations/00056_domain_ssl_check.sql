-- +goose Up
-- +goose StatementBegin

-- Marketer audit 2026-10-06, #4: a domain's SSL status was only ever set by
-- a Cloudflare check, so every other domain showed "none" for good. The
-- worker now opens a real TLS connection to each domain (internal/sslcheck)
-- and records what it found: the served certificate's expiry, when it last
-- looked, and why the check failed. In production a TLS-terminating proxy
-- with on-demand certificates (Caddy, docs/deployment.md) issues the
-- certificate on the first such connection, so the check also triggers it.
ALTER TABLE domains
  ADD COLUMN ssl_expires_at timestamptz,
  ADD COLUMN ssl_checked_at timestamptz,
  ADD COLUMN ssl_error      text NOT NULL DEFAULT '';

-- The check rewrites ssl_checked_at on every domain every few minutes. The
-- tracker reads only domains.domain (routingstore.ResolveTrackingLink), so
-- only a write that can change it needs to flush the tracker's routing
-- cache.
DROP TRIGGER domains_routing_changed ON domains;
CREATE TRIGGER domains_routing_changed
  AFTER INSERT OR DELETE OR TRUNCATE ON domains
  FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER domains_routing_changed_update
  AFTER UPDATE OF domain ON domains
  FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER domains_routing_changed_update ON domains;
DROP TRIGGER domains_routing_changed ON domains;
CREATE TRIGGER domains_routing_changed
  AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON domains
  FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
ALTER TABLE domains
  DROP COLUMN ssl_error,
  DROP COLUMN ssl_checked_at,
  DROP COLUMN ssl_expires_at;
-- +goose StatementEnd

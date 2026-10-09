-- +goose Up
-- +goose StatementBegin

-- Tech audit 2026-10-06, T6. A domain tracked as soon as any organization
-- typed it in — status was client input, default 'active' — and the name
-- was unique across all organizations, so whoever added someone else's
-- domain first held it: the owner could not add it at all. Now a domain
-- becomes active only when its organization proves control of its DNS
-- (a TXT record carrying verification_token, internal/domain), and only
-- active domains are unique and tracked.
ALTER TABLE domains
  ADD COLUMN verification_token text NOT NULL DEFAULT replace(gen_random_uuid()::text, '-', ''),
  ADD COLUMN ownership_verified_at timestamptz;

-- Domains already active stay active: they were set up before this check
-- existed (no production deployment yet).
UPDATE domains SET ownership_verified_at = created_at WHERE status = 'active';

DROP INDEX domains_domain_key;
-- One active owner per name; any number of organizations may have it
-- pending, so a squatter's entry blocks nobody.
CREATE UNIQUE INDEX domains_active_domain_key ON domains (lower(domain)) WHERE status = 'active';
-- An organization lists a name once.
CREATE UNIQUE INDEX domains_org_domain_key ON domains (organization_id, lower(domain));

-- The tracker now reads status too (routingstore.ResolveTrackingLink):
-- a domain verified or taken off active must flush its routing cache.
DROP TRIGGER domains_routing_changed_update ON domains;
CREATE TRIGGER domains_routing_changed_update
  AFTER UPDATE OF domain, status ON domains
  FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER domains_routing_changed_update ON domains;
CREATE TRIGGER domains_routing_changed_update
  AFTER UPDATE OF domain ON domains
  FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
DROP INDEX domains_org_domain_key;
DROP INDEX domains_active_domain_key;
-- Fails if two organizations now hold the same name.
CREATE UNIQUE INDEX domains_domain_key ON domains (lower(domain));
ALTER TABLE domains DROP COLUMN ownership_verified_at, DROP COLUMN verification_token;
-- +goose StatementEnd

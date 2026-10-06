-- +goose Up
-- +goose StatementBegin

-- Audit 2026-10-06, RL1/A1: apps/tracker now serves clicks from an
-- in-memory copy of the routing configuration (internal/routingcache) and
-- must hear about every edit at once. A trigger per routing table — rather
-- than a call in each API write path — cannot be forgotten by the next
-- endpoint that writes one of these tables.
--
-- One statement-level NOTIFY per write: the tracker flushes its whole cache
-- on any notification, so the payload (the table name) is informational.
-- NOTIFY is transactional: listeners hear it only after the edit commits,
-- and never for a rolled-back one.
CREATE FUNCTION flox_notify_routing_changed() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_notify('flox_routing_changed', TG_TABLE_NAME);
  RETURN NULL;
END;
$$;

CREATE TRIGGER domains_routing_changed           AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON domains           FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER tracking_links_routing_changed    AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON tracking_links    FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER campaigns_routing_changed         AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON campaigns         FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER stream_sets_routing_changed       AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON stream_sets       FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER filter_groups_routing_changed     AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON filter_groups     FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER filter_conditions_routing_changed AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON filter_conditions FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER flows_routing_changed             AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON flows             FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER offers_routing_changed            AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON offers            FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER offer_links_routing_changed       AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON offer_links       FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER landings_routing_changed          AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON landings          FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();
CREATE TRIGGER pwas_routing_changed              AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON pwas              FOR EACH STATEMENT EXECUTE FUNCTION flox_notify_routing_changed();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER pwas_routing_changed              ON pwas;
DROP TRIGGER landings_routing_changed          ON landings;
DROP TRIGGER offer_links_routing_changed       ON offer_links;
DROP TRIGGER offers_routing_changed            ON offers;
DROP TRIGGER flows_routing_changed             ON flows;
DROP TRIGGER filter_conditions_routing_changed ON filter_conditions;
DROP TRIGGER filter_groups_routing_changed     ON filter_groups;
DROP TRIGGER stream_sets_routing_changed       ON stream_sets;
DROP TRIGGER campaigns_routing_changed         ON campaigns;
DROP TRIGGER tracking_links_routing_changed    ON tracking_links;
DROP TRIGGER domains_routing_changed           ON domains;
DROP FUNCTION flox_notify_routing_changed();
-- +goose StatementEnd

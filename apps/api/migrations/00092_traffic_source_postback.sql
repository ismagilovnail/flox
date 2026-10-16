-- Postback to the traffic source (audit 2026-10-15, INT-1;
-- docs/traffic-sources.md "Postback to the source").
--
-- An ad network (PropellerAds, ExoClick, Adsterra…) optimizes on the
-- conversions it is told about. A source's postback_url is called, macro
-- resolved, when a conversion attributed to one of its clicks is recorded
-- with a status in postback_statuses — through the same delivery queue as a
-- network's outgoing postback (postback_deliveries, now either a network's
-- or a source's). template_key names the catalog template the source was
-- set up from (apps/web lib/source-templates.ts), for the link builder.

-- +goose Up
ALTER TABLE traffic_sources
  ADD COLUMN postback_url text NOT NULL DEFAULT '',
  ADD COLUMN postback_statuses text[] NOT NULL DEFAULT '{CPA_ACCEPT}',
  ADD COLUMN template_key text NOT NULL DEFAULT '';

ALTER TABLE postback_deliveries
  ALTER COLUMN network_id DROP NOT NULL,
  ADD COLUMN traffic_source_id ulid REFERENCES traffic_sources(id) ON DELETE CASCADE,
  ADD CONSTRAINT postback_deliveries_one_target
    CHECK ((network_id IS NULL) <> (traffic_source_id IS NULL));

-- +goose Down
DELETE FROM postback_deliveries WHERE traffic_source_id IS NOT NULL;
ALTER TABLE postback_deliveries
  DROP CONSTRAINT postback_deliveries_one_target,
  DROP COLUMN traffic_source_id,
  ALTER COLUMN network_id SET NOT NULL;
ALTER TABLE traffic_sources
  DROP COLUMN template_key,
  DROP COLUMN postback_statuses,
  DROP COLUMN postback_url;

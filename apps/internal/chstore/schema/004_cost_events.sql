-- Table 4 of 5: ad spend, mirroring Postgres's cost_entries (migration
-- 00009) for efficient cross-database-free JOINs against click/conversion
-- volume at query time (CLAUDE.md #6, §27-COST). Postgres stays the
-- source of truth for cost entry — this table is a denormalized copy for
-- analytics, the same relationship ClickHouse has to every other Postgres-
-- authoritative entity.
--
-- Schema only in this phase: the sync pipeline from cost_entries into this
-- table is Phase 27-COST's job ("Cost ingestion," ROADMAP.md), the same
-- way cost_entries itself started manual-entry-only in Phase 17 before any
-- FB/TikTok import existed. An empty table with the right shape is not a
-- fake API that looks real — nothing queries it yet, and CLAUDE.md #6's
-- "no cost for a slice shows ROI as '—', never zero" already holds
-- trivially for a table with zero rows.
--
-- One row per (organization_id, date, campaign_id, traffic_source_id) —
-- cost_entries' own natural key, per docs/domain-model.md.
CREATE TABLE IF NOT EXISTS cost_events
(
    organization_id   String,
    date              Date,
    campaign_id       String,
    traffic_source_id String,
    country           LowCardinality(String),

    spend             Float64,
    currency          LowCardinality(String),
    spend_usd         Float64,
    has_usd_value     UInt8, -- see conversion_events: "no FX rate" != "zero"

    updated_at        DateTime64(3, 'UTC') -- mirrors cost_entries.updated_at, for sync freshness
)
ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY toYYYYMM(date)
ORDER BY (organization_id, date, campaign_id, traffic_source_id)
SETTINGS index_granularity = 8192;

-- Table 5 of 5: the rich per-attempt postback log migration 00008 (Phase
-- 17) earmarked for here — "does NOT duplicate the rich per-attempt log...
-- that belongs in ClickHouse's postback_events, outside this Postgres-only
-- phase." Both directions land in one table, discriminated by `direction`:
--
--   incoming: a network calling FLOX's /postback/{networkId} (§45,
--     internal/conversion). result is success/duplicate/ignored/error.
--   outgoing: FLOX calling a network's postback_url (§46,
--     internal/postback). result is queued/processing/success/failed/
--     retrying/dead, and attempt_count/response_status_code/url are only
--     ever set here — incoming attempts leave them at their zero value.
--
-- This table is NOT the dedup/delivery source of truth — Postgres's
-- `postbacks` and `postback_deliveries` still are (CLAUDE.md #3's
-- correctness constraint lives there, enforced by a real unique
-- constraint and FOR UPDATE SKIP LOCKED). This is the read-side replay/
-- audit log §45 requires ("log every postback... with replay ability"),
-- fed asynchronously off the same durable-queue pattern as click/tracking/
-- conversion events — never on the critical path of either direction.
--
-- ORDER BY (organization_id, event_at, network_id): "every attempt for
-- this network, this period" is the dominant query (a network's own
-- dashboard, or debugging one integration). click_id gets a bloom filter
-- instead of a sort-key slot — "every attempt for this click" (a disputed
-- payout, docs/attribution.md's framing) is a rarer, point-lookup-shaped
-- query that doesn't need range-scan locality the way network_id does.
--
-- No TTL — see click_events' comment.
CREATE TABLE IF NOT EXISTS postback_events
(
    organization_id String,
    event_at DateTime64(3, 'UTC'),
    direction LowCardinality(String), -- incoming | outgoing

    network_id String,
    click_id String,
    status LowCardinality(String), -- CPA_HOLD .. CPA_TRASH, or empty (incoming error before mapping)
    event_ref String,
    raw_status String, -- incoming only: the network's own status string before mapping

    result LowCardinality(String), -- incoming: success/duplicate/ignored/error; outgoing: queued/processing/success/failed/retrying/dead
    message String,
    attempt_count Int64, -- outgoing only
    response_status_code Int64, -- outgoing only; 0 means no HTTP response was ever received
    url String, -- outgoing only: the macro-resolved URL actually dispatched

    revenue Float64, -- incoming only
    currency LowCardinality(String),

    INDEX idx_click_id click_id TYPE bloom_filter(0.01) GRANULARITY 4
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (organization_id, event_at, network_id, direction)
SETTINGS index_granularity = 8192;

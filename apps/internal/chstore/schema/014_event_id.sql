-- Audit 2026-10-06: every event carries a stable id (event.Event.ID) from
-- its creation through every retry; for a conversion it is the postbacks
-- row id. The reconciliation (internal/reconcile) matches and repairs
-- conversions by it. Appended last, after 008-011's columns —
-- chstore.EventStore appends positionally. Rows from before this have ''.
ALTER TABLE click_events      ADD COLUMN IF NOT EXISTS event_id String;
ALTER TABLE tracking_events   ADD COLUMN IF NOT EXISTS event_id String;
ALTER TABLE conversion_events ADD COLUMN IF NOT EXISTS event_id String;
ALTER TABLE ltv_events        ADD COLUMN IF NOT EXISTS event_id String;

-- ltv_events' view (schema/007 drops the older ones): CPA statuses including
-- declines, so a chargeback — a decline carrying the reversed accept's
-- payout, negated — nets out of LTV (T4/AN3); event_id so a repaired
-- conversion's LTV row can be found and replaced. On the start that
-- upgrades an existing store, a conversion inserted in the milliseconds
-- between 007's drop and this create gets no LTV row.
CREATE MATERIALIZED VIEW IF NOT EXISTS ltv_events_mv_v3
TO ltv_events
AS
SELECT
    organization_id, click_id, campaign_id, network_id, country, event_at, type, usd_value, has_usd_value, event_id
FROM conversion_events
WHERE type IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH');

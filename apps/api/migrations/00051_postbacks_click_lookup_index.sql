-- +goose NO TRANSACTION

-- Audit 2026-10-06, DB1: every incoming postback reads its click's latest
-- successful status (conversion.lastStatus, twice: the fast pre-check and
-- under the click lock) and replays look up the source row
-- (FindSuccessID). postbacks had only (organization_id) and (network_id)
-- indexes plus the partial dedup key — which the planner cannot use for
-- these queries, since they do not repeat its NOT network_accepts_duplicates
-- predicate — so each postback filtered every postback of its organization.
--
-- CONCURRENTLY, outside a transaction: on a live database this does not
-- block writes to postbacks while it builds. IF NOT EXISTS so a retry after
-- an interrupted build can run again (an interrupted CONCURRENTLY build
-- leaves an INVALID index — drop it first in that case).

-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS postbacks_click_success_idx
  ON postbacks (organization_id, click_id, created_at DESC)
  WHERE result = 'success';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS postbacks_click_success_idx;

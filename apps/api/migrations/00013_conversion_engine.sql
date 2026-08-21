-- +goose Up
-- +goose StatementBegin

-- Phase 23 (§45) implements the conversion engine as CODE and brings the
-- 00008 postbacks table's dedup key up to the A1/A2 spec amendment it
-- predates: (organization_id, click_id, status) is NOT the correct key —
-- it would drop every redeposit after the first. The real key is
-- (organization_id, click_id, status, event_ref); see internal/conversion
-- for the event_ref rules (empty for every status except CPA_REDEP).

DROP INDEX postbacks_dedup_key;

ALTER TABLE postbacks
  -- Dedup key's third component. Empty string for every status except
  -- CPA_REDEP (§45) — NOT NULL with a default so the unique index below
  -- treats "no event_ref" uniformly instead of NULL's "distinct from
  -- itself" behavior silently disabling dedup for non-REDEP statuses.
  ADD COLUMN event_ref          text NOT NULL DEFAULT '',
  -- The network's raw transaction id, stored on every row regardless of
  -- whether this status's dedup key uses it (§45: "still STORED on the
  -- event").
  ADD COLUMN network_txn_id     text NOT NULL DEFAULT '',
  -- The network's own status string before Event Mapping translated it —
  -- kept for the postback log / replay story even when mapping fails.
  ADD COLUMN raw_status         text NOT NULL DEFAULT '',
  ADD COLUMN revenue            numeric(14, 4),
  ADD COLUMN currency           text NOT NULL DEFAULT '',
  -- usd_value is nullable and independent of "revenue IS NULL": a
  -- postback can report revenue in a currency with no FX rate on file yet,
  -- and that must not be confused with "converts to zero" (CLAUDE.md #7).
  ADD COLUMN usd_value          numeric(14, 4),
  ADD COLUMN attribution_outcome text NOT NULL DEFAULT '',
  ADD COLUMN attributed_click_id text NOT NULL DEFAULT '',
  ADD COLUMN attribution_method  text NOT NULL DEFAULT '',
  ADD COLUMN time_to_conversion_ms bigint,
  ADD COLUMN message             text NOT NULL DEFAULT '';

-- 'ignored' is §45's STATUS PROGRESSION outcome: a postback that was
-- understood, logged in full, and deliberately not applied because it
-- would have moved a conversion back to CPA_HOLD. Distinct from 'error'
-- (couldn't understand/process it) and 'duplicate' (dedup key already
-- seen).
ALTER TABLE postbacks
  DROP CONSTRAINT postbacks_result_check,
  ADD CONSTRAINT postbacks_result_check CHECK (result IN ('success', 'duplicate', 'error', 'ignored'));

-- 00008's status CHECK required one of the five CpaStatus values on every
-- row. A 'result = error' row can legitimately have no canonical status at
-- all — a postback missing click_id/status entirely, or one whose raw
-- status has no Event Mapping configured — and forcing a fabricated status
-- onto it would misrepresent what happened. Only 'error' rows get this
-- exemption; every other result still requires a real CpaStatus.
ALTER TABLE postbacks
  DROP CONSTRAINT postbacks_status_check,
  ADD CONSTRAINT postbacks_status_check CHECK (
    status IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH')
    OR (status = '' AND result = 'error')
  );

-- Scoped to result = 'success' (in addition to the existing accept_duplicates
-- exemption) so this table can hold one row per INCOMING POSTBACK ATTEMPT —
-- success, duplicate, ignored, and error alike, per §45's "log every
-- postback... with replay ability" — while still only ever having ONE
-- 'success' row per dedup key. A duplicate/ignored/error attempt gets its
-- own row (own id, own created_at) precisely because it does NOT compete
-- for this index; only a second 'success' row for the same key would.
CREATE UNIQUE INDEX postbacks_dedup_key ON postbacks (organization_id, click_id, status, event_ref)
  WHERE NOT network_accepts_duplicates AND result = 'success';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX postbacks_dedup_key;

ALTER TABLE postbacks
  DROP CONSTRAINT postbacks_status_check,
  ADD CONSTRAINT postbacks_status_check CHECK (status IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH'));

ALTER TABLE postbacks
  DROP CONSTRAINT postbacks_result_check,
  ADD CONSTRAINT postbacks_result_check CHECK (result IN ('success', 'duplicate', 'error'));

ALTER TABLE postbacks
  DROP COLUMN event_ref,
  DROP COLUMN network_txn_id,
  DROP COLUMN raw_status,
  DROP COLUMN revenue,
  DROP COLUMN currency,
  DROP COLUMN usd_value,
  DROP COLUMN attribution_outcome,
  DROP COLUMN attributed_click_id,
  DROP COLUMN attribution_method,
  DROP COLUMN time_to_conversion_ms,
  DROP COLUMN message;

CREATE UNIQUE INDEX postbacks_dedup_key ON postbacks (organization_id, click_id, status)
  WHERE NOT network_accepts_duplicates;

-- +goose StatementEnd

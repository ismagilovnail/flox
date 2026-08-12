-- +goose Up
-- +goose StatementBegin

CREATE TABLE pixels (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name            text NOT NULL,
  provider        text NOT NULL CHECK (provider IN ('facebook', 'tiktok', 'snapchat', 'twitter', 'generic')),
  pixel_id        text NOT NULL, -- the provider's own pixel identifier
  -- Curated subset of the §43 event model a pixel can fire on — same
  -- text[] rationale as postlandings.events.
  events          text[] NOT NULL DEFAULT '{}',
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX pixels_organization_id_idx ON pixels (organization_id);

CREATE TRIGGER pixels_set_updated_at
  BEFORE UPDATE ON pixels
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Many-to-many: a stream set fires zero or more pixels (frontend
-- StreamSet.pixels: string[]). Junction table rather than an array column
-- since pixel_id needs a real FK (arrays can't reference other tables).
CREATE TABLE stream_set_pixels (
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  stream_set_id   ulid NOT NULL REFERENCES stream_sets (id) ON DELETE CASCADE,
  pixel_id        ulid NOT NULL REFERENCES pixels (id) ON DELETE CASCADE,
  PRIMARY KEY (stream_set_id, pixel_id)
);

CREATE INDEX stream_set_pixels_organization_id_idx ON stream_set_pixels (organization_id);
CREATE INDEX stream_set_pixels_pixel_id_idx ON stream_set_pixels (pixel_id);

-- The durable half of §45's postback dedup rule: "dedup key = (click_id,
-- status), NOT click_id alone... Long Redis TTL + durable DB unique
-- constraint." Redis (once wired up, later phase) is the fast-path cache;
-- this table is the source of truth Redis is caching, so a Redis flush can
-- never reopen a dedup window. It intentionally does NOT duplicate the
-- rich per-attempt log (message, raw payload, request/response) that
-- belongs in ClickHouse's postback_events (high-volume, analytics-shaped,
-- outside Phase 17's Postgres-only scope) — this table only needs to
-- answer "have we already processed (click_id, status) for this org."
CREATE TABLE postbacks (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  network_id      ulid NOT NULL REFERENCES networks (id),
  click_id        text NOT NULL, -- opaque id minted by the tracker (apps/tracker, Phase 21) — not a Postgres FK, clicks live in ClickHouse
  status          text NOT NULL CHECK (status IN ('CPA_HOLD', 'CPA_ACCEPT', 'CPA_REDEP', 'CPA_DECLINE', 'CPA_TRASH')),
  direction       text NOT NULL CHECK (direction IN ('incoming', 'outgoing')),
  result          text NOT NULL CHECK (result IN ('success', 'duplicate', 'error')),
  -- Snapshot of networks.accept_duplicates at write time, so the dedup
  -- constraint below can exempt networks whose re-send semantics require
  -- accepting repeats (§45) — a partial index can't reach across to the
  -- networks table, so the flag has to live on the row it's guarding.
  network_accepts_duplicates boolean NOT NULL DEFAULT false,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX postbacks_organization_id_idx ON postbacks (organization_id);
CREATE INDEX postbacks_network_id_idx ON postbacks (network_id);
-- The dedup constraint itself (§45, non-negotiable invariant #3) — skipped
-- for networks with the accept_duplicates override.
CREATE UNIQUE INDEX postbacks_dedup_key ON postbacks (organization_id, click_id, status)
  WHERE NOT network_accepts_duplicates;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE postbacks;
DROP TABLE stream_set_pixels;
DROP TABLE pixels;
-- +goose StatementEnd

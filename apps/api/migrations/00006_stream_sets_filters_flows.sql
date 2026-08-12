-- +goose Up
-- +goose StatementBegin

CREATE TABLE stream_sets (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  campaign_id     ulid NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
  name            text NOT NULL,
  -- First match wins (§21/§39): lower priority number evaluates first.
  priority        integer NOT NULL DEFAULT 0,
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused')),
  fallback_url    text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX stream_sets_organization_id_idx ON stream_sets (organization_id);
-- The routing engine's hot-path query: "this campaign's active stream sets,
-- in evaluation order."
CREATE INDEX stream_sets_campaign_id_priority_idx ON stream_sets (campaign_id, priority);

CREATE TRIGGER stream_sets_set_updated_at
  BEFORE UPDATE ON stream_sets
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- A stream set's filter is a tree (AND/OR groups nesting conditions and
-- other groups — frontend FilterGroupNode/FilterCondition in lib/filters.ts).
-- stream_set_id is denormalized onto every node (not just the root) so any
-- node can be tenant-scoped and queried directly without walking the tree
-- up to find which stream set it belongs to.
CREATE TABLE filter_groups (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  stream_set_id   ulid NOT NULL REFERENCES stream_sets (id) ON DELETE CASCADE,
  parent_group_id ulid REFERENCES filter_groups (id) ON DELETE CASCADE, -- NULL = this is the stream set's root group
  joiner          text NOT NULL CHECK (joiner IN ('AND', 'OR')),
  position        integer NOT NULL DEFAULT 0, -- order among sibling groups under the same parent
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX filter_groups_organization_id_idx ON filter_groups (organization_id);
CREATE INDEX filter_groups_stream_set_id_idx ON filter_groups (stream_set_id);
CREATE INDEX filter_groups_parent_group_id_idx ON filter_groups (parent_group_id);
-- Exactly one root group per stream set.
CREATE UNIQUE INDEX filter_groups_one_root_per_stream_set
  ON filter_groups (stream_set_id) WHERE parent_group_id IS NULL;

CREATE TABLE filter_conditions (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  filter_group_id ulid NOT NULL REFERENCES filter_groups (id) ON DELETE CASCADE,
  position        integer NOT NULL DEFAULT 0, -- order among sibling conditions/groups under the same parent
  field           text NOT NULL, -- frontend FilterField, e.g. "country", "sub1" (§5/§22)
  operator        text NOT NULL, -- frontend FilterOperator, e.g. "IS", "CONTAINS", "BETWEEN"
  value           text NOT NULL DEFAULT '',
  value_to        text NOT NULL DEFAULT '', -- BETWEEN's upper bound only
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX filter_conditions_organization_id_idx ON filter_conditions (organization_id);
CREATE INDEX filter_conditions_filter_group_id_idx ON filter_conditions (filter_group_id);

-- Weighted destinations under a stream set (§24: pickWeighted). The
-- landing/pwa/postlanding "stage" and "destination" structs from frontend
-- Flow are flattened into nullable columns gated by their own *_enabled /
-- destination_kind flag — a small fixed set of variants, not worth a
-- separate table per stage.
CREATE TABLE flows (
  id                    ulid PRIMARY KEY,
  organization_id       ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  stream_set_id         ulid NOT NULL REFERENCES stream_sets (id) ON DELETE CASCADE,
  name                  text NOT NULL,
  active                boolean NOT NULL DEFAULT true,
  weight                integer NOT NULL DEFAULT 100, -- raw integer, normalized to % at read time (§24)
  position              integer NOT NULL DEFAULT 0,

  landing_enabled       boolean NOT NULL DEFAULT false,
  landing_id            ulid REFERENCES landings (id),
  landing_as_pwa        boolean NOT NULL DEFAULT false,

  pwa_enabled           boolean NOT NULL DEFAULT false,
  pwa_id                ulid REFERENCES pwas (id),
  pwa_type              text CHECK (pwa_type IN ('internal', 'external', 'ios_app')),

  postlanding_enabled   boolean NOT NULL DEFAULT false,
  postlanding_id        ulid REFERENCES postlandings (id),

  destination_kind      text NOT NULL CHECK (destination_kind IN ('offer', 'redirect')),
  destination_network_id ulid REFERENCES networks (id),
  destination_offer_id  ulid REFERENCES offers (id),
  destination_url       text,

  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT flows_destination_shape CHECK (
    (destination_kind = 'offer' AND destination_network_id IS NOT NULL AND destination_offer_id IS NOT NULL AND destination_url IS NULL)
    OR
    (destination_kind = 'redirect' AND destination_url IS NOT NULL AND destination_network_id IS NULL AND destination_offer_id IS NULL)
  )
);

CREATE INDEX flows_organization_id_idx ON flows (organization_id);
CREATE INDEX flows_stream_set_id_idx ON flows (stream_set_id);

CREATE TRIGGER flows_set_updated_at
  BEFORE UPDATE ON flows
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE flows;
DROP TABLE filter_conditions;
DROP TABLE filter_groups;
DROP TABLE stream_sets;
-- +goose StatementEnd

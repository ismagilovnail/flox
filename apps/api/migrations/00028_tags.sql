-- +goose Up
-- +goose StatementBegin

-- Tags (§30.6) — a cross-entity color-label system spanning Campaigns, CPA
-- Networks, Offers, Flows, Traffic Sources, PWA apps, and Landing Pages.
-- Frontend has carried the full UX (features/tags/*) against a zustand
-- mock store (stores/tags.ts) since its own frontend-first phase; this
-- migration + apps/internal/tags is the real backend replacing it.
--
-- Generic `tags` table + a polymorphic `taggables` join, exactly the
-- data model the spec itself names. `entity_type` is a fixed CHECK
-- whitelist matching the frontend's own TaggableEntityType union — the
-- seven entities above, nothing else (Postlanding/Pixels/Conversions/
-- Domains/Team are explicitly not taggable per spec).
CREATE TABLE tags (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name            text NOT NULL,
  color           text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tags_organization_id_idx ON tags (organization_id);

CREATE TRIGGER tags_set_updated_at
  BEFORE UPDATE ON tags
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- organization_id is denormalized onto the join row itself (not just
-- reachable via tag_id) so every taggables query stays a direct,
-- single-table tenant filter (§36-TENANCY) rather than a join through
-- tags for every read. entity_id is NOT a foreign key — it's polymorphic
-- across seven different tables, so referential integrity for it is
-- enforced in the service layer (apps/internal/tags validates the id
-- belongs to a real, same-org row of that entity_type before writing),
-- the same pattern already used for domain_id/campaign_id on tracking
-- links. The unique constraint stops the same tag being assigned twice
-- to the same entity; ON DELETE CASCADE on tag_id means deleting a tag
-- removes it from everywhere it was assigned, per spec.
CREATE TABLE taggables (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  tag_id          ulid NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
  entity_type     text NOT NULL CHECK (entity_type IN ('campaign', 'network', 'offer', 'flow', 'traffic_source', 'pwa', 'landing')),
  entity_id       ulid NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tag_id, entity_type, entity_id)
);

CREATE INDEX taggables_organization_id_idx ON taggables (organization_id);
CREATE INDEX taggables_org_entity_idx ON taggables (organization_id, entity_type, entity_id);
CREATE INDEX taggables_tag_id_idx ON taggables (tag_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE taggables;
DROP TABLE tags;
-- +goose StatementEnd

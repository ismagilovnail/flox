-- +goose Up
-- +goose StatementBegin

CREATE TABLE landings (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name            text NOT NULL,
  type            text NOT NULL CHECK (type IN ('internal', 'external')),
  url             text NOT NULL DEFAULT '', -- CDN URL for internal, advertiser's own URL for external
  content         text NOT NULL DEFAULT '', -- page HTML, internal only
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX landings_organization_id_idx ON landings (organization_id);

CREATE TRIGGER landings_set_updated_at
  BEFORE UPDATE ON landings
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE pwas (
  id                     ulid PRIMARY KEY,
  organization_id        ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name                   text NOT NULL,
  short_name             text NOT NULL,
  theme_color            text NOT NULL,
  background_color       text NOT NULL,
  icon_url               text NOT NULL,
  start_url              text NOT NULL,
  -- §73: bouncing in-app WebView (FB/IG/TikTok/Telegram) to the external
  -- browser so the install prompt can fire — provider-neutral, required.
  bounce_in_app_webview  boolean NOT NULL DEFAULT true,
  status                 text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX pwas_organization_id_idx ON pwas (organization_id);

CREATE TRIGGER pwas_set_updated_at
  BEFORE UPDATE ON pwas
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE postlandings (
  id              ulid PRIMARY KEY,
  organization_id ulid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
  name            text NOT NULL,
  url             text NOT NULL,
  -- Curated subset of the §43 event model a postlanding can plausibly fire
  -- on (PWA_INSTALL, NOTIFICATION_*, TG_JOIN, TG_START) — stored as text[]
  -- rather than a FK table since it's a small fixed vocabulary, same
  -- reasoning as mock/postlandings.ts.
  events          text[] NOT NULL DEFAULT '{}',
  status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX postlandings_organization_id_idx ON postlandings (organization_id);

CREATE TRIGGER postlandings_set_updated_at
  BEFORE UPDATE ON postlandings
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE postlandings;
DROP TABLE pwas;
DROP TABLE landings;
-- +goose StatementEnd

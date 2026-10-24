-- Clicks over the tracker's per-IP limit (audit REL-2,
-- docs/security.md "Rate limiting" / internal/clicklimit).
--
-- Such a visitor is sent on unrecorded (audit SEC-4), so no report counts
-- them; apps/tracker adds them up per campaign and UTC day in memory and
-- writes the totals here every 30 s, for the campaign page's warning.
-- Rows are kept 90 days (the tracker deletes older ones).
--
-- No foreign key to campaigns: a count for a campaign deleted meanwhile
-- must not fail the tracker's batch; the organization's cascade clears
-- the rest.

-- +goose Up
CREATE TABLE ip_limited_clicks (
  organization_id ulid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  campaign_id     ulid NOT NULL,
  day             date NOT NULL,
  clicks          bigint NOT NULL CHECK (clicks > 0),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, campaign_id, day)
);

-- +goose Down
DROP TABLE ip_limited_clicks;

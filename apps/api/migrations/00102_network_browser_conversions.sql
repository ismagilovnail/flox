-- Conversions reported by a page's script (audit UNI-1 part 2,
-- docs/networks-offers.md "Own site without a server").
--
-- A site without a server (Tilda and other builders) cannot call the
-- secret postback URL. When the operator turns this on for a network,
-- apps/tracker accepts GET/POST /c/{networkId} with no secret — only from
-- pages on browser_domains (their subdomains included), only statuses that
-- map to CPA_HOLD or CPA_ACCEPT, only for a click that exists, once each.

-- +goose Up
ALTER TABLE networks
  ADD COLUMN browser_conversions boolean NOT NULL DEFAULT false,
  ADD COLUMN browser_domains text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE networks DROP COLUMN browser_conversions, DROP COLUMN browser_domains;

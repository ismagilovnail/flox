/** Settings (§30) — integrations only now. Organization (name/timezone/
 * currency), API keys, and Security (2FA/session timeout/IP allowlist)
 * all got real backends — see lib/organization.ts, lib/api-keys.ts,
 * lib/security.ts, hooks/use-{organization,api-keys,security}.ts, plus
 * hooks/use-auth.ts's TOTP hooks for real per-user 2FA enrollment.
 * Custom metrics is intentionally NOT here — it's its own real feature
 * (apps/internal/custommetrics), separate from Settings even though §30
 * mentions it.
 *
 * Integrations (registrar/DNS/ad-platform/Slack OAuth) remain mock-only —
 * real Facebook/TikTok Ads OAuth connections DO exist for real
 * (apps/internal/adaccount), but they live under Traffic Sources' own
 * per-source connection UI, not this org-level panel; the other four
 * providers here (Google Ads, Namecheap, Cloudflare, Slack) have no
 * backend anywhere. A real candidate for its own later phase.
 */

export type IntegrationProvider = "facebook_ads" | "tiktok_ads" | "google_ads" | "namecheap" | "cloudflare" | "slack";

export type IntegrationStatus = "connected" | "not_connected" | "error";

export type Integration = {
  id: string;
  provider: IntegrationProvider;
  label: string;
  description: string;
  status: IntegrationStatus;
  connectedAt: string | null;
};

export const INTEGRATIONS: Integration[] = [
  {
    id: "int_facebook_ads",
    provider: "facebook_ads",
    label: "Facebook Ads",
    description: "Pull ad spend automatically for ROI (§27-COST). OAuth wiring lands in Phase 27-COST.",
    status: "not_connected",
    connectedAt: null,
  },
  {
    id: "int_tiktok_ads",
    provider: "tiktok_ads",
    label: "TikTok Ads",
    description: "Pull ad spend automatically for ROI (§27-COST). OAuth wiring lands in Phase 27-COST.",
    status: "not_connected",
    connectedAt: null,
  },
  {
    id: "int_google_ads",
    provider: "google_ads",
    label: "Google Ads",
    description: "Pull ad spend automatically for ROI (§27-COST).",
    status: "not_connected",
    connectedAt: null,
  },
  {
    id: "int_namecheap",
    provider: "namecheap",
    label: "Namecheap",
    description: "Registrar connection for automatic domain renewal and DNS record management (§30).",
    status: "connected",
    connectedAt: "2026-02-05T00:00:00Z",
  },
  {
    id: "int_cloudflare",
    provider: "cloudflare",
    label: "Cloudflare",
    description: "DNS + automatic SSL issuance for tracking/PWA/fallback domains (§30).",
    status: "connected",
    connectedAt: "2026-01-18T00:00:00Z",
  },
  {
    id: "int_slack",
    provider: "slack",
    label: "Slack",
    description: "Post campaign and team alerts to a channel.",
    status: "not_connected",
    connectedAt: null,
  },
];

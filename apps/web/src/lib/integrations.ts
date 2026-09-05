/**
 * Settings > Integrations (§30) — real vocabulary for both halves of the
 * panel: apps/internal/integrations' three credential-based providers
 * (Namecheap, Cloudflare, Slack) and the read-only ad-platform aggregate
 * it computes over apps/internal/adaccount's own real per-traffic-source
 * data. Facebook Ads/TikTok Ads/Google Ads deliberately have no connect
 * form here — that's Traffic Sources' own job (features/traffic-sources/
 * ad-account-connection-section.tsx); this panel only ever shows real
 * status for them, it never duplicates the connection flow.
 */

export type CredentialProvider = "namecheap" | "cloudflare" | "slack";

export type IntegrationStatus = "connected" | "error" | "not_connected";

export type Integration = {
  provider: CredentialProvider;
  status: IntegrationStatus;
  lastVerifiedAt: string | null;
  lastError: string | null;
};

export type AdPlatformStatus = {
  provider: string;
  connectedCount: number;
  totalCount: number;
};

export type IntegrationsSummary = {
  integrations: Integration[];
  adPlatforms: AdPlatformStatus[];
};

export type CredentialField = {
  name: string;
  label: string;
  type: "text" | "password";
  placeholder?: string;
};

export type CredentialProviderMeta = {
  provider: CredentialProvider;
  label: string;
  description: string;
  fields: CredentialField[];
};

// fields here must match apps/internal/integrations.Provider.RequiredFields
// exactly (name-for-name) — the two are the frontend/backend halves of the
// same contract, same tradeoff the report-builder dimension whitelist and
// custom-metrics registry already accept elsewhere in this codebase.
export const CREDENTIAL_PROVIDERS: CredentialProviderMeta[] = [
  {
    provider: "namecheap",
    label: "Namecheap",
    description: "Registrar connection for automatic domain renewal and DNS record management (§30).",
    fields: [
      { name: "apiUser", label: "API user", type: "text" },
      { name: "apiKey", label: "API key", type: "password" },
      { name: "clientIp", label: "Whitelisted client IP", type: "text", placeholder: "203.0.113.10" },
    ],
  },
  {
    provider: "cloudflare",
    label: "Cloudflare",
    description: "DNS + automatic SSL issuance for tracking/PWA/fallback domains (§30).",
    fields: [{ name: "apiToken", label: "API token", type: "password" }],
  },
  {
    provider: "slack",
    label: "Slack",
    description: "Post campaign and team alerts to a channel via an incoming webhook.",
    fields: [{ name: "webhookUrl", label: "Incoming webhook URL", type: "text", placeholder: "https://hooks.slack.com/services/..." }],
  },
];

export const AD_PLATFORM_LABELS: Record<string, string> = {
  facebook_ads: "Facebook Ads",
  tiktok_ads: "TikTok Ads",
  google_ads: "Google Ads",
};

export const AD_PLATFORM_DESCRIPTIONS: Record<string, string> = {
  facebook_ads: "Pull ad spend automatically for ROI (§27-COST). Connected per traffic source.",
  tiktok_ads: "Pull ad spend automatically for ROI (§27-COST). Connected per traffic source.",
  google_ads: "Pull ad spend automatically for ROI (§27-COST). Connected per traffic source.",
};

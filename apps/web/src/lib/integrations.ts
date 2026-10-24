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

/** Labels and descriptions live in the `settings` translations, keyed by
 * provider and field name (integrations.providers.* / integrations.fields.*). */
export type CredentialField = {
  name: string;
  type: "text" | "password";
  placeholder?: string;
};

export type CredentialProviderMeta = {
  provider: CredentialProvider;
  label: string;
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
    fields: [
      { name: "apiUser", type: "text" },
      { name: "apiKey", type: "password" },
      { name: "clientIp", type: "text", placeholder: "203.0.113.10" },
    ],
  },
  {
    provider: "cloudflare",
    label: "Cloudflare",
    fields: [{ name: "apiToken", type: "password" }],
  },
  {
    provider: "slack",
    label: "Slack",
    fields: [{ name: "webhookUrl", type: "text", placeholder: "https://hooks.slack.com/services/..." }],
  },
];

export const AD_PLATFORM_LABELS: Record<string, string> = {
  facebook_ads: "Facebook Ads",
  tiktok_ads: "TikTok Ads",
  google_ads: "Google Ads",
  yandex_direct: "Yandex Direct",
  vk_ads: "VK Ads",
};

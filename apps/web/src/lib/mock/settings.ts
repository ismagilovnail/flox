/** Settings (§30) — organization, API keys, integrations, security. Custom
 * metrics is intentionally NOT here — it's Phase 14.6, a separate later
 * phase per the CLAUDE.md build order, even though §30 mentions it. */

export type OrgSettings = {
  name: string;
  timezone: string;
  currency: string;
};

export const TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Los_Angeles",
  "America/Chicago",
  "Europe/London",
  "Europe/Berlin",
  "Asia/Dubai",
  "Asia/Singapore",
];

/** Matches the mock signed-in workspace in components/shell/workspace-selector.tsx. */
export const ORG_SETTINGS: OrgSettings = {
  name: "Nail Ismagilov",
  timezone: "UTC",
  currency: "USD",
};

export type ApiKeyScope = "read" | "write" | "admin";
export const API_KEY_SCOPES: ApiKeyScope[] = ["read", "write", "admin"];

export type ApiKey = {
  id: string;
  name: string;
  /** Only the prefix is ever persisted/displayed after creation — the full key is shown once. */
  prefix: string;
  scope: ApiKeyScope;
  createdAt: string;
  lastUsedAt: string | null;
};

export const API_KEYS: ApiKey[] = [
  {
    id: "key_reporting",
    name: "Reporting export",
    prefix: "flx_live_8f3a...",
    scope: "read",
    createdAt: "2026-03-01T00:00:00Z",
    lastUsedAt: "2026-08-10T09:00:00Z",
  },
  {
    id: "key_zapier",
    name: "Zapier webhook relay",
    prefix: "flx_live_2c91...",
    scope: "write",
    createdAt: "2026-05-14T00:00:00Z",
    lastUsedAt: "2026-07-22T14:30:00Z",
  },
];

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

export type SecuritySettings = {
  twoFactorRequired: boolean;
  sessionTimeoutMinutes: number;
  ipAllowlist: string[];
};

export const SECURITY_SETTINGS: SecuritySettings = {
  twoFactorRequired: false,
  sessionTimeoutMinutes: 720,
  ipAllowlist: [],
};

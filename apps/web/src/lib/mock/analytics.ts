import type { DimensionKey } from "@/features/analytics/registry";

/**
 * Report builder's filter-value dropdown now suggests from the real
 * backend (POST /analytics/dimension-values, hooks/use-analytics-
 * report.ts's useDimensionValues) for the 14 dimensions
 * apps/internal/chstore.ValidDimension backs. This pool now serves two
 * narrower roles: the brief loading-state placeholder before that query
 * resolves, and the only source at all for the 3 still-unbacked
 * dimensions (pwa/postlanding/network), whose query always
 * rejects with a validation error by design (no backing column exists
 * yet — report-controls.tsx's own AddFilterPopover catches that and
 * falls back to this). Filtering by an arbitrary value still round-trips
 * to the real API correctly regardless of its source; a value that never
 * occurs in real event data just yields a legitimate empty report, not
 * an error.
 */
const POOLS: Record<DimensionKey, string[]> = {
  campaign: [
    "US Sweeps — FB",
    "UK Nutra — TikTok",
    "DE Dating — Push",
    "AU Sweeps — FB",
    "CA Crypto — Native",
  ],
  source: ["Facebook", "TikTok", "Google", "Native Ads", "Push", "SEO"],
  country: ["US", "UK", "DE", "AU", "CA", "FR", "BR"],
  region: ["California", "Texas", "New York", "Ontario", "Bavaria", "Normandy"],
  city: ["Los Angeles", "New York", "London", "Berlin", "Toronto", "Paris", "Sao Paulo"],
  device: ["Mobile", "Desktop", "Tablet"],
  platform: ["iOS", "Android", "Windows", "macOS", "Linux"],
  os: ["iOS 17", "iOS 18", "Android 13", "Android 14", "Windows 11", "macOS 15"],
  browser: ["Chrome", "Safari", "Firefox", "Edge", "Samsung Internet"],
  language: ["en", "de", "fr", "es", "pt"],
  flow: ["Tier-1 English 70/30", "EU Nutra Weighted", "Push Fallback", "Native Priority"],
  stream_set: ["Tier-1 Mobile", "EU Desktop", "Fallback Catch-all", "iOS Only"],
  landing: ["LP Sweeps A", "LP Sweeps B", "LP Nutra A", "LP Dating A"],
  pwa: ["PWA Sweeps", "PWA Nutra", "None"],
  postlanding: ["PL Upsell A", "PL Survey B", "None"],
  // Matches the real Offer/Network entities (mock/offers.ts, mock/networks.ts) so
  // "View statistics" (§27.5) drill-in from those list pages lands on non-empty data.
  offer: ["US Sweeps — CPA $12", "UK Nutra Trial", "DE Dating — CPL", "CA Crypto — RevShare"],
  network: ["AffTrust CPA", "AdCombo", "MyLead", "Direct advertiser"],
};

export function dimensionValues(key: DimensionKey): string[] {
  return POOLS[key];
}

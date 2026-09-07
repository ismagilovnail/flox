import type { DimensionKey } from "@/features/analytics/registry";

/**
 * Report builder's filter-value dropdown still suggests from this fixed
 * pool rather than the real backend — apps/internal/chstore/report.go
 * (Phase 35) has no "distinct values seen for dimension X" endpoint, only
 * equals-filtering, and building that is a separate, undertaken-later
 * feature (see CHANGELOG's Phase 35 entry, Known issues). Filtering by an
 * arbitrary value still round-trips to the real API correctly; a value
 * that never occurs in real event data just yields a legitimate empty
 * report, not an error.
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

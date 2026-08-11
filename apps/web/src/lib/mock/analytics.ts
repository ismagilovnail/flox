import type { DimensionKey } from "@/features/analytics/registry";

function mulberry32(seed: number) {
  return function rand() {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

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
  landing: ["LP Sweeps A", "LP Sweeps B", "LP Nutra A", "LP Dating A"],
  pwa: ["PWA Sweeps", "PWA Nutra", "None"],
  postlanding: ["PL Upsell A", "PL Survey B", "None"],
  offer: ["Sweeps Gold US", "Nutra Slim UK", "Dating Prime DE", "Crypto Wallet AU"],
  network: ["MaxBounty", "PropellerAds", "ClickDealer", "AdCombo"],
};

export function dimensionValues(key: DimensionKey): string[] {
  return POOLS[key];
}

export type RawSlice = Record<DimensionKey, string> & {
  date: string;
  clicks: number;
  uniqueClicks: number;
  landClicks: number;
  conversions: number;
  revenue: number;
  cost: number;
  /** false = no cost data for this slice at all (§27-COST), distinct from a genuine $0 spend. */
  hasCost: boolean;
};

function pick<T>(rand: () => number, arr: T[]): T {
  return arr[Math.floor(rand() * arr.length)];
}

function round2(n: number) {
  return Math.round(n * 100) / 100;
}

export function generateAnalyticsSlices(days = 60, count = 700): RawSlice[] {
  const rand = mulberry32(9110226);
  const today = new Date("2026-08-11T00:00:00Z");
  const keys = DIMENSION_KEYS;

  return Array.from({ length: count }, () => {
    const dayOffset = Math.floor(rand() * days);
    const d = new Date(today);
    d.setUTCDate(d.getUTCDate() - dayOffset);

    const dims = Object.fromEntries(
      keys.map((k) => [k, pick(rand, POOLS[k])]),
    ) as Record<DimensionKey, string>;

    const clicks = Math.round(20 + rand() * 380);
    const uniqueClicks = Math.round(clicks * (0.75 + rand() * 0.2));
    const landClicks = Math.round(clicks * (0.3 + rand() * 0.4));
    const cvr = 0.01 + rand() * 0.05;
    const conversions = Math.round(uniqueClicks * cvr);
    const revenue = round2(conversions * (10 + rand() * 30));
    // ~15% of slices carry no cost data at all (§27-COST: must render as "—", not 0).
    const hasCost = rand() > 0.15;
    const cost = hasCost ? round2(revenue * (0.3 + rand() * 0.5)) : 0;

    return {
      ...dims,
      date: d.toISOString().slice(0, 10),
      clicks,
      uniqueClicks,
      landClicks,
      conversions,
      revenue,
      cost,
      hasCost,
    } satisfies RawSlice;
  });
}

const DIMENSION_KEYS: DimensionKey[] = [
  "campaign",
  "source",
  "country",
  "region",
  "city",
  "device",
  "platform",
  "os",
  "browser",
  "language",
  "flow",
  "landing",
  "pwa",
  "postlanding",
  "offer",
  "network",
];

/** Full event-model funnel (§43) — SOURCE_CLICK is the funnel's implicit base (= clicks). */
export const FUNNEL_STEPS = [
  "SOURCE_CLICK",
  "LAND_VIEW",
  "LAND_CLICK",
  "PWA_VIEW",
  "PWA_INSTALL",
  "CPA_HOLD",
  "CPA_ACCEPT",
  "CPA_REDEP",
] as const;

export function generateFunnelMock() {
  const rand = mulberry32(4471);
  let value = 200_000 + Math.floor(rand() * 20_000);
  const dropoffs = [0, 0.18, 0.35, 0.42, 0.55, 0.4, 0.3, 0.6];
  return FUNNEL_STEPS.map((step, i) => {
    if (i > 0) value = Math.round(value * (1 - dropoffs[i] - rand() * 0.08));
    return { step, value };
  });
}

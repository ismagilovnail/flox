function mulberry32(seed: number) {
  return function rand() {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export type CampaignStatus = "active" | "paused" | "draft" | "archived";

export type Campaign = {
  id: string;
  name: string;
  status: CampaignStatus;
  source: string;
  trackingDomain: string;
  trackingId: string;
  fallbackUrl: string;
  notes: string;
  createdAt: string;
  updatedAt: string;
  clicks: number;
  conversions: number;
  revenue: number;
  spend: number | null;
};

export type DailyPoint = { date: string; revenue: number; spend: number; clicks: number; conversions: number };

export const SOURCES = ["Facebook", "TikTok", "Google", "Native Ads", "Push", "SEO"];
export const TRACKING_DOMAINS = ["track.floxlink.io", "go.floxtrk.com", "clk.floxdsp.net"];

const NAMES = [
  "US Sweeps — FB",
  "UK Nutra — TikTok",
  "DE Dating — Push",
  "AU Sweeps — FB",
  "CA Crypto — Native",
  "FR Nutra — Google",
  "BR Sweeps — Native",
  "US Dating — TikTok",
  "UK Crypto — Push",
  "DE Sweeps — FB",
  "CA Nutra — SEO",
  "AU Dating — Google",
  "US Crypto — Native",
  "FR Sweeps — Push",
];

const STATUS_CYCLE: CampaignStatus[] = ["active", "active", "active", "paused", "draft", "archived"];

function round2(n: number) {
  return Math.round(n * 100) / 100;
}

function genId(rand: () => number) {
  const chars = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
  let out = "";
  for (let i = 0; i < 12; i++) out += chars[Math.floor(rand() * chars.length)];
  return out;
}

export function generateCampaigns(): Campaign[] {
  const rand = mulberry32(778241);
  const today = new Date("2026-08-11T00:00:00Z");

  return NAMES.map((name, i) => {
    const clicks = Math.round(4000 + rand() * 25000);
    const cvr = 0.015 + rand() * 0.04;
    const conversions = Math.round(clicks * cvr);
    const revenue = round2(conversions * (12 + rand() * 25));
    const hasCost = rand() > 0.12;
    const spend = hasCost ? round2(revenue * (0.35 + rand() * 0.45)) : null;
    const createdDaysAgo = Math.round(10 + rand() * 300);
    const updatedDaysAgo = Math.round(rand() * createdDaysAgo);
    const created = new Date(today);
    created.setUTCDate(created.getUTCDate() - createdDaysAgo);
    const updated = new Date(today);
    updated.setUTCDate(updated.getUTCDate() - updatedDaysAgo);

    return {
      id: genId(rand),
      name,
      status: STATUS_CYCLE[i % STATUS_CYCLE.length],
      source: SOURCES[Math.floor(rand() * SOURCES.length)],
      trackingDomain: TRACKING_DOMAINS[Math.floor(rand() * TRACKING_DOMAINS.length)],
      trackingId: genId(rand).slice(0, 8),
      fallbackUrl: "https://example.com/offer-fallback",
      notes: "",
      createdAt: created.toISOString(),
      updatedAt: updated.toISOString(),
      clicks,
      conversions,
      revenue,
      spend,
    };
  });
}

/** Deterministic per-campaign daily series for the detail-page trend chart. */
export function generateCampaignDaily(campaignId: string, days = 30): DailyPoint[] {
  let seed = 0;
  for (let i = 0; i < campaignId.length; i++) seed = (seed * 31 + campaignId.charCodeAt(i)) | 0;
  const rand = mulberry32(seed);
  const today = new Date("2026-08-11T00:00:00Z");

  return Array.from({ length: days }, (_, i) => {
    const d = new Date(today);
    d.setUTCDate(d.getUTCDate() - (days - 1 - i));
    const clicks = Math.round(150 + rand() * 900);
    const cvr = 0.02 + rand() * 0.04;
    const conversions = Math.round(clicks * cvr);
    const revenue = round2(conversions * (12 + rand() * 20));
    const spend = round2(revenue * (0.4 + rand() * 0.4));
    return { date: d.toISOString().slice(0, 10), revenue, spend, clicks, conversions };
  });
}

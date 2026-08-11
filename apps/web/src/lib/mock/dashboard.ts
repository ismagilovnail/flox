/**
 * Deterministic mock data (seeded PRNG, not Math.random) so static-exported
 * pages render identical output on every build and hydrate without mismatch.
 */

function mulberry32(seed: number) {
  return function rand() {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export type DailyPoint = {
  date: string;
  revenue: number;
  spend: number;
  profit: number;
  clicks: number;
  conversions: number;
};

/** null spend/roi models "no cost data for this slice" (§27-COST) — must render as "—", never 0. */
export type PerformanceRow = {
  id: string;
  name: string;
  clicks: number;
  conversions: number;
  cvr: number;
  revenue: number;
  spend: number | null;
  roi: number | null;
};

export type DashboardMock = {
  daily: DailyPoint[];
  topCampaigns: PerformanceRow[];
  topOffers: PerformanceRow[];
  topCountries: PerformanceRow[];
  topFlows: PerformanceRow[];
};

function round2(n: number) {
  return Math.round(n * 100) / 100;
}

function makeRow(
  rand: () => number,
  id: string,
  name: string,
  scale: number,
  spendless = false,
): PerformanceRow {
  const clicks = Math.round((400 + rand() * 4000) * scale);
  const cvr = 0.015 + rand() * 0.05;
  const conversions = Math.round(clicks * cvr);
  const revenue = round2(conversions * (8 + rand() * 30));
  if (spendless) {
    return { id, name, clicks, conversions, cvr, revenue, spend: null, roi: null };
  }
  const spend = round2(revenue * (0.4 + rand() * 0.5));
  const roi = spend > 0 ? round2(((revenue - spend) / spend) * 100) : null;
  return { id, name, clicks, conversions, cvr, revenue, spend, roi };
}

export function generateDashboardMock(days = 30): DashboardMock {
  const rand = mulberry32(20260811);
  const today = new Date("2026-08-11T00:00:00Z");

  const daily: DailyPoint[] = Array.from({ length: days }, (_, i) => {
    const d = new Date(today);
    d.setUTCDate(d.getUTCDate() - (days - 1 - i));
    const weekday = d.getUTCDay();
    const weekendDip = weekday === 0 || weekday === 6 ? 0.8 : 1;
    const trend = 1 + i / (days * 3.2);
    const noise = 0.85 + rand() * 0.3;
    const clicks = Math.round(3200 * weekendDip * trend * noise);
    const cvr = 0.028 + rand() * 0.012;
    const conversions = Math.round(clicks * cvr);
    const revenue = round2(conversions * (14 + rand() * 6));
    const spend = round2(revenue * (0.45 + rand() * 0.2));
    const profit = round2(revenue - spend);
    return {
      date: d.toISOString().slice(0, 10),
      revenue,
      spend,
      profit,
      clicks,
      conversions,
    };
  });

  const topCampaigns: PerformanceRow[] = [
    makeRow(rand, "CMP-1042", "US Sweeps — FB", 1.4),
    makeRow(rand, "CMP-1041", "UK Nutra — TikTok", 1.1),
    makeRow(rand, "CMP-1039", "DE Dating — Push", 0.7),
    makeRow(rand, "CMP-1030", "AU Sweeps — FB", 1.6),
    makeRow(rand, "CMP-1028", "CA Crypto — Native", 0.5, true),
  ].sort((a, b) => b.revenue - a.revenue);

  const topOffers: PerformanceRow[] = [
    makeRow(rand, "OFR-201", "Sweeps Gold US", 1.5),
    makeRow(rand, "OFR-198", "Nutra Slim UK", 1.0),
    makeRow(rand, "OFR-187", "Dating Prime DE", 0.6),
    makeRow(rand, "OFR-176", "Crypto Wallet AU", 0.9),
  ].sort((a, b) => b.revenue - a.revenue);

  const topCountries: PerformanceRow[] = [
    makeRow(rand, "US", "United States", 2.1),
    makeRow(rand, "GB", "United Kingdom", 1.0),
    makeRow(rand, "DE", "Germany", 0.8),
    makeRow(rand, "AU", "Australia", 0.7),
    makeRow(rand, "CA", "Canada", 0.6),
  ].sort((a, b) => b.revenue - a.revenue);

  const topFlows: PerformanceRow[] = [
    makeRow(rand, "FLW-501", "Tier-1 English 70/30", 1.3),
    makeRow(rand, "FLW-498", "EU Nutra Weighted", 0.9),
    makeRow(rand, "FLW-487", "Push Fallback", 0.5),
    makeRow(rand, "FLW-476", "Native Priority", 0.8),
  ].sort((a, b) => b.revenue - a.revenue);

  return { daily, topCampaigns, topOffers, topCountries, topFlows };
}

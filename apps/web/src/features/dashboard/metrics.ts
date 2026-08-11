import type { DailyPoint } from "@/lib/mock/dashboard";

export type PeriodMetrics = {
  revenue: number;
  spend: number;
  profit: number;
  roi: number | null;
  clicks: number;
  conversions: number;
  cvr: number;
  cpa: number | null;
};

export function aggregate(points: DailyPoint[]): PeriodMetrics {
  const revenue = points.reduce((s, p) => s + p.revenue, 0);
  const spend = points.reduce((s, p) => s + p.spend, 0);
  const clicks = points.reduce((s, p) => s + p.clicks, 0);
  const conversions = points.reduce((s, p) => s + p.conversions, 0);
  const profit = revenue - spend;
  return {
    revenue,
    spend,
    profit,
    roi: spend > 0 ? (profit / spend) * 100 : null,
    clicks,
    conversions,
    cvr: clicks > 0 ? conversions / clicks : 0,
    cpa: conversions > 0 ? spend / conversions : null,
  };
}

/** Percent change, current vs previous. Null when there's no baseline to compare against. */
export function pctDelta(curr: number, prev: number): number | null {
  if (prev === 0) return null;
  return ((curr - prev) / prev) * 100;
}

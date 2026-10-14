import type { DimensionKey, MetricKey } from "@/features/analytics/registry";
import type { ReportRow } from "@/features/analytics/aggregate";

/** null spend/roi models "no cost data for this slice" (§27-COST) — must render as "—", never 0. */
export type PerformanceRow = {
  id: string;
  name: string;
  clicks: number;
  conversions: number;
  cvr: number;
  /** null = revenue on a day with no reporting-currency rate (§50-FX) — "—". */
  revenue: number | null;
  spend: number | null;
  roi: number | null;
};

export type PeriodMetrics = {
  /** null = revenue on a day with no reporting-currency rate (§50-FX) — "—". */
  revenue: number | null;
  /** null = no cost_entries at all for this range (CLAUDE.md #6) — "—", never a fabricated $0. */
  spend: number | null;
  profit: number | null;
  roi: number | null;
  clicks: number;
  conversions: number;
  cvr: number;
  cpa: number | null;
  /** Clicks no destination took (SOURCE_FILTER) and clicks flagged as bots; the
   * rates are null when there was no traffic at all — "—", never 0. */
  blocked: number;
  bots: number;
  blockedRate: number | null;
  botRate: number | null;
};

/** Maps apps/internal/analytics's §50 metrics registry row (Phase 35) onto
 * the dashboard's own narrower KPI shape — cvr comes back as a fraction
 * here (KpiGrid's own `pct()` multiplies by 100), matching MetricValues.CVR
 * being a percentage on the wire. spend/profit/roi/cpa all stay null exactly
 * where CLAUDE.md #6 says "no cost data on file" must render as "—", never
 * a fabricated 0 — none of the four mean anything without a real cost row. */
export function metricsFromApi(m: Record<MetricKey, number | null>): PeriodMetrics {
  return {
    revenue: m.revenue,
    spend: m.cost,
    profit: m.profit,
    roi: m.roi,
    clicks: m.clicks ?? 0,
    conversions: m.conversions ?? 0,
    cvr: (m.cvr ?? 0) / 100,
    cpa: m.cpa,
    blocked: m.blocked ?? 0,
    bots: m.bots ?? 0,
    blockedRate: m.blockedRate,
    botRate: m.botRate,
  };
}

/** Maps one /overview breakdown row (Phase 35's Dashboard endpoint groups
 * by exactly one dimension — DimCampaign/DimCountry/DimFlow) onto TopTable's
 * row shape. `id`/`name` are both the raw dimension value: DimCampaign/
 * DimFlow resolve to campaign_id/flow_id (chstore's own whitelist, no name
 * join yet — a real but honest gap, see CHANGELOG's Phase 35 entry rather
 * than a fabricated display name). */
export function performanceRowFromApi(
  row: ReportRow,
  dim: DimensionKey,
  /** Display name for the dimension value (campaign/flow names); defaults to the raw value. */
  label: (dim: DimensionKey, value: string) => string = (_d, v) => v,
): PerformanceRow {
  const value = row.dims[dim] ?? "";
  return {
    id: value,
    name: label(dim, value),
    clicks: row.metrics.clicks ?? 0,
    conversions: row.metrics.conversions ?? 0,
    cvr: (row.metrics.cvr ?? 0) / 100,
    revenue: row.metrics.revenue,
    spend: row.metrics.cost,
    roi: row.metrics.roi,
  };
}

/** Percent change, current vs previous. Null when there's no baseline to compare against. */
export function pctDelta(curr: number, prev: number): number | null {
  if (prev === 0) return null;
  return ((curr - prev) / prev) * 100;
}

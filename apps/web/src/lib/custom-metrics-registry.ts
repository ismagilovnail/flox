/**
 * Custom Metrics catalog (§30.5) — extends the ONE metrics registry
 * (lib/analytics-registry.ts, which already implements §50 and is
 * reused here verbatim, not duplicated) with the categories §50 documents
 * that Phase 5 didn't need: CPA Funnel, Push, and LTV.
 *
 * CPA Funnel is catalog-only for now (`live: false`): no surface exposes a
 * formula built from those ids. Push is live: GET /push/stats and the
 * analytics report return these ids per row, so push formulas evaluate on
 * Push > Statistics (push_stats) and in the report builder. LTV is catalog-only AND
 * `insertable: false` — §30.5 explicitly forbids LTV in custom formulas, not
 * just "not implemented yet".
 *
 * Real domain vocabulary (mirrored server-side by apps/internal/
 * custommetrics/registry.go, which validates every formula against the
 * same catalog before it's ever saved) — not mock data, so it doesn't
 * live under lib/mock/.
 */

import { METRICS, isPushMetric } from "@/lib/analytics-registry";
import type { MetricFormat, MetricKey } from "@/types/analytics";

export type MetricCategory = "Traffic / Performance" | "CPA Funnel" | "Push" | "Fraud" | "LTV";
export type MetricDataSource = "regular" | "push" | "ltv";

export type RegistryMetric = {
  id: string;
  label: string;
  category: MetricCategory;
  dataSource: MetricDataSource;
  format: MetricFormat;
  insertable: boolean;
  /** Whether any surface can currently compute this metric's live value. */
  live: boolean;
};

const TRAFFIC_PERFORMANCE: RegistryMetric[] = METRICS.filter((m) => !isPushMetric(m.key)).map((m) => ({
  id: m.key,
  label: m.label,
  category: "Traffic / Performance",
  dataSource: "regular",
  format: m.format,
  insertable: true,
  live: true,
}));

const CPA_FUNNEL_BASE: { id: string; label: string; format: MetricFormat }[] = [
  { id: "cpa_hold", label: "CPA Hold (registrations)", format: "int" },
  { id: "cpa_accept", label: "CPA Accept (First Purchases)", format: "int" },
  { id: "cpa_redep", label: "CPA Repeat Purchase", format: "int" },
  { id: "cpa_decline", label: "CPA Decline", format: "int" },
  { id: "cpa_trash", label: "CPA Trash", format: "int" },
  { id: "reg_to_ftd_rate", label: "Reg → First Purchase Rate", format: "percent" },
  { id: "ftd_to_redep_rate", label: "First → Repeat Purchase Rate", format: "percent" },
  { id: "dep_to_redep", label: "Purchase → Repeat Purchase Rate", format: "percent" },
  { id: "total_deposits", label: "Total Purchases", format: "int" },
  { id: "total_deposit_revenue", label: "Total Purchase Revenue", format: "currency" },
];
const CPA_FUNNEL: RegistryMetric[] = CPA_FUNNEL_BASE.map((m) => ({
  ...m,
  category: "CPA Funnel" as const,
  dataSource: "regular" as const,
  insertable: true,
  live: false,
}));

// "bots" is a report metric (Traffic / Performance above); only "all clicks
// incl. bots" stays catalog-only.
const FRAUD: RegistryMetric[] = [
  { id: "click_all", label: "All Clicks (incl. bots)" },
].map((m) => ({ ...m, category: "Fraud" as const, dataSource: "regular" as const, format: "int" as MetricFormat, insertable: true, live: false }));

// The report's push columns (the same ids Push > Statistics rows carry).
const PUSH: RegistryMetric[] = METRICS.filter((m) => isPushMetric(m.key)).map((m) => ({
  id: m.key,
  label: m.label,
  format: m.format,
  category: "Push" as const,
  dataSource: "push" as const,
  insertable: true,
  live: true,
}));

const LTV: RegistryMetric[] = [
  { id: "ltv_d0", label: "LTV Day 0" },
  { id: "ltv_d1_7", label: "LTV Day 1-7" },
  { id: "ltv_d8_30", label: "LTV Day 8-30" },
  { id: "ltv_d31_90", label: "LTV Day 31-90" },
  { id: "ltv_total", label: "LTV Total" },
  { id: "ltv_per_ftd", label: "LTV per First Purchase" },
  { id: "ltv_per_reg", label: "LTV per Registration" },
  { id: "lifetime_days", label: "Lifetime Days" },
].map((m) => ({
  ...m,
  category: "LTV" as const,
  dataSource: "ltv" as const,
  format: (m.id === "lifetime_days" ? "int" : "currency") as MetricFormat,
  insertable: false,
  live: false,
}));

export const METRIC_CATALOG: RegistryMetric[] = [...TRAFFIC_PERFORMANCE, ...CPA_FUNNEL, ...FRAUD, ...PUSH, ...LTV];

export const METRIC_CATEGORIES: MetricCategory[] = ["Traffic / Performance", "CPA Funnel", "Fraud", "Push", "LTV"];

export function catalogMetric(id: string): RegistryMetric | undefined {
  return METRIC_CATALOG.find((m) => m.id === id);
}

export type ShowInTarget = "report_builder" | "campaigns_table" | "offers_table" | "sources_table" | "push_stats";

export const SHOW_IN_TARGETS: { id: ShowInTarget; label: string }[] = [
  { id: "report_builder", label: "Report Builder" },
  { id: "campaigns_table", label: "Campaigns Table" },
  { id: "offers_table", label: "Offers Table" },
  { id: "sources_table", label: "Traffic Sources Table" },
  { id: "push_stats", label: "Push Statistics" },
];

/** Which registry metric ids each surface can actually supply as inputs — drives
 * the §30.5 "disabled when a formula input isn't available" rule. Offers/Sources
 * carry no analytics numbers yet (no tracker), so those two are honestly almost
 * always disabled rather than faked. */
export const SURFACE_AVAILABLE_METRIC_IDS: Record<ShowInTarget, Set<MetricKey | string>> = {
  report_builder: new Set(METRICS.map((m) => m.key)),
  campaigns_table: new Set(["clicks", "conversions", "revenue", "cost", "profit", "roi", "emptySubsRate"]),
  offers_table: new Set(),
  sources_table: new Set(),
  push_stats: new Set(["push_sent", "push_delivered", "push_opened", "push_unsubscribed", "push_ctr"]),
};

export function surfaceCanCompute(target: ShowInTarget, usedMetricIds: string[]): boolean {
  const available = SURFACE_AVAILABLE_METRIC_IDS[target];
  return usedMetricIds.length > 0 && usedMetricIds.every((id) => available.has(id));
}

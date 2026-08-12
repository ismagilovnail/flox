/**
 * Custom Metrics catalog (§30.5) — extends the ONE metrics registry
 * (features/analytics/registry.ts, which already implements §50 and is
 * reused here verbatim, not duplicated) with the categories §50 documents
 * that Phase 5 didn't need: CPA Funnel, Push, and LTV.
 *
 * CPA Funnel and Push are catalog-only for now (`live: false`) — there's no
 * tracker or Push module yet to compute them, so no surface can expose a
 * formula built from them until those phases land. LTV is catalog-only AND
 * `insertable: false` — §30.5 explicitly forbids LTV in custom formulas, not
 * just "not implemented yet".
 */

import { METRICS, type MetricFormat, type MetricKey } from "@/features/analytics/registry";

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

const TRAFFIC_PERFORMANCE: RegistryMetric[] = METRICS.map((m) => ({
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
  { id: "cpa_accept", label: "CPA Accept (FTDs)", format: "int" },
  { id: "cpa_redep", label: "CPA Redeposit", format: "int" },
  { id: "cpa_decline", label: "CPA Decline", format: "int" },
  { id: "cpa_trash", label: "CPA Trash", format: "int" },
  { id: "reg_to_ftd_rate", label: "Reg → FTD Rate", format: "percent" },
  { id: "ftd_to_redep_rate", label: "FTD → Redep Rate", format: "percent" },
  { id: "dep_to_redep", label: "Deposit → Redeposit Rate", format: "percent" },
  { id: "total_deposits", label: "Total Deposits", format: "int" },
  { id: "total_deposit_revenue", label: "Total Deposit Revenue", format: "currency" },
];
const CPA_FUNNEL: RegistryMetric[] = CPA_FUNNEL_BASE.map((m) => ({
  ...m,
  category: "CPA Funnel" as const,
  dataSource: "regular" as const,
  insertable: true,
  live: false,
}));

const FRAUD: RegistryMetric[] = [
  { id: "bots", label: "Bot Clicks" },
  { id: "click_all", label: "All Clicks (incl. bots)" },
].map((m) => ({ ...m, category: "Fraud" as const, dataSource: "regular" as const, format: "int" as MetricFormat, insertable: true, live: false }));

const PUSH_BASE: { id: string; label: string; format: MetricFormat }[] = [
  { id: "push_sent", label: "Push Sent", format: "int" },
  { id: "push_delivered", label: "Push Delivered", format: "int" },
  { id: "push_opened", label: "Push Opened", format: "int" },
  { id: "push_ctr", label: "Push CTR", format: "percent" },
];
const PUSH: RegistryMetric[] = PUSH_BASE.map((m) => ({
  ...m,
  category: "Push" as const,
  dataSource: "push" as const,
  insertable: true,
  live: false,
}));

const LTV: RegistryMetric[] = [
  { id: "ltv_d0", label: "LTV Day 0" },
  { id: "ltv_d1_7", label: "LTV Day 1-7" },
  { id: "ltv_d8_30", label: "LTV Day 8-30" },
  { id: "ltv_d31_90", label: "LTV Day 31-90" },
  { id: "ltv_total", label: "LTV Total" },
  { id: "ltv_per_ftd", label: "LTV per FTD" },
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

export type ShowInTarget = "report_builder" | "campaigns_table" | "offers_table" | "sources_table";

export const SHOW_IN_TARGETS: { id: ShowInTarget; label: string }[] = [
  { id: "report_builder", label: "Report Builder" },
  { id: "campaigns_table", label: "Campaigns Table" },
  { id: "offers_table", label: "Offers Table" },
  { id: "sources_table", label: "Traffic Sources Table" },
];

/** Which registry metric ids each surface can actually supply as inputs — drives
 * the §30.5 "disabled when a formula input isn't available" rule. Offers/Sources
 * carry no analytics numbers yet (no tracker), so those two are honestly almost
 * always disabled rather than faked. */
export const SURFACE_AVAILABLE_METRIC_IDS: Record<ShowInTarget, Set<MetricKey | string>> = {
  report_builder: new Set(METRICS.map((m) => m.key)),
  campaigns_table: new Set(["clicks", "conversions", "revenue", "cost", "profit", "roi"]),
  offers_table: new Set(),
  sources_table: new Set(),
};

export function surfaceCanCompute(target: ShowInTarget, usedMetricIds: string[]): boolean {
  const available = SURFACE_AVAILABLE_METRIC_IDS[target];
  return usedMetricIds.length > 0 && usedMetricIds.every((id) => available.has(id));
}

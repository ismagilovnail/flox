import { apiFetch } from "@/lib/api/client";
import type { DimensionKey, MetricKey } from "@/features/analytics/registry";
import type { FilterCondition, ReportRow } from "@/features/analytics/aggregate";

/** Mirrors apps/internal/analytics's response shapes exactly. */

/** Which day a conversion is reported on: the day the network reported it,
 * or the day of the click it is attributed to (tech audit AN5). */
export type DateBasis = "conversion" | "click";
export type DailyCount = { day: string; type: string; eventCount: number };
export type DailyRevenue = { day: string; type: string; eventCount: number; revenueUsd: number };

function formatDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

export function getCampaignDaily(campaignId: string, from: Date, to: Date): Promise<{ counts: DailyCount[] }> {
  return apiFetch(`/analytics/campaigns/${campaignId}/daily`, {
    searchParams: { from: formatDate(from), to: formatDate(to) },
  });
}

export function getCampaignDailyRevenue(
  campaignId: string,
  from: Date,
  to: Date,
  dateBasis: DateBasis = "conversion",
): Promise<{ revenue: DailyRevenue[] }> {
  return apiFetch(`/analytics/campaigns/${campaignId}/daily-revenue`, {
    searchParams: { from: formatDate(from), to: formatDate(to), dateBasis },
  });
}

// --- Phase 35: Report/Timeseries/Funnel/Dashboard ---
// Mirrors apps/internal/analytics/handler.go's metricValuesResponse/
// reportRowResponse exactly — one metric-values shape reused by every
// endpoint below, same as the Go side.

type ApiMetrics = Record<MetricKey, number | null>;

type ApiReportRow = { day?: string; dims?: Record<string, string>; metrics: ApiMetrics; datedByConversion?: number };

function toReportRow(row: ApiReportRow, dims: DimensionKey[]): ReportRow {
  const dimValues = { ...(row.dims ?? {}) } as Partial<Record<DimensionKey, string>>;
  // "day" is not a column the API groups by: the row carries it as `day`, and
  // it is folded into dims so the table treats it like any other dimension.
  if (row.day && dims.includes("day")) dimValues.day = row.day;
  const key =
    dims.length === 0 ? (row.day ?? "__all__") : dims.map((d) => dimValues[d] ?? "").join(" · ");
  return { key, dims: dimValues, metrics: row.metrics, datedByConversion: row.datedByConversion };
}

function toFilterBody(filters: FilterCondition[]) {
  return filters.map((f) => ({ dimension: f.dimension, value: f.value }));
}

export type TimeseriesPoint = { day: string; metrics: ApiMetrics };

export function getReport(input: {
  dims: DimensionKey[];
  filters: FilterCondition[];
  from: Date;
  to: Date;
  sort: { key: string; dir: "asc" | "desc" };
  timezone?: string;
  dateBasis?: DateBasis;
}): Promise<ReportRow[]> {
  return apiFetch<{ rows: ApiReportRow[] }>("/analytics/report", {
    method: "POST",
    body: {
      dims: input.dims,
      filters: toFilterBody(input.filters),
      from: formatDate(input.from),
      to: formatDate(input.to),
      sort: input.sort,
      timezone: input.timezone,
      dateBasis: input.dateBasis,
    },
  }).then((res) => res.rows.map((r) => toReportRow(r, input.dims)));
}

export function getTimeseries(input: {
  filters: FilterCondition[];
  from: Date;
  to: Date;
  timezone?: string;
  dateBasis?: DateBasis;
}): Promise<TimeseriesPoint[]> {
  return apiFetch<{ rows: ApiReportRow[] }>("/analytics/timeseries", {
    method: "POST",
    body: {
      filters: toFilterBody(input.filters),
      from: formatDate(input.from),
      to: formatDate(input.to),
      timezone: input.timezone,
      dateBasis: input.dateBasis,
    },
  }).then((res) => res.rows.map((r) => ({ day: r.day!, metrics: r.metrics })));
}

export type FunnelStepData = { step: string; value: number };

export function getFunnel(
  from: Date,
  to: Date,
  filters: FilterCondition[] = [],
  timezone?: string,
): Promise<FunnelStepData[]> {
  return apiFetch<{ steps: FunnelStepData[] }>("/analytics/funnel", {
    method: "POST",
    body: { from: formatDate(from), to: formatDate(to), filters: toFilterBody(filters), timezone },
  }).then((res) => res.steps);
}

/**
 * Real distinct values seen for one dimension — backs the report
 * builder's filter-value dropdown. Rejects (ApiError, status 400) a
 * dimension apps/internal/chstore.ValidDimension does not back; the
 * dropdown then offers nothing (never a made-up value), see
 * hooks/use-analytics-report.ts's useDimensionValues.
 */
export function getDimensionValues(dim: DimensionKey, from: Date, to: Date): Promise<string[]> {
  return apiFetch<{ values: string[] }>("/analytics/dimension-values", {
    method: "POST",
    body: { dim, from: formatDate(from), to: formatDate(to) },
  }).then((res) => res.values);
}

export type DashboardReport = {
  totals: ApiMetrics;
  /** Dated by click: conversions in the totals with no attributed click,
   * counted on their own date. 0 otherwise. */
  datedByConversion: number;
  daily: TimeseriesPoint[];
  topCampaigns: ReportRow[];
  topCountries: ReportRow[];
  topFlows: ReportRow[];
  topOffers: ReportRow[];
};

export function getDashboardReport(
  from: Date,
  to: Date,
  timezone?: string,
  dateBasis: DateBasis = "conversion",
): Promise<DashboardReport> {
  return apiFetch<{
    totals: ApiMetrics;
    datedByConversion?: number;
    daily: ApiReportRow[];
    topCampaigns: ApiReportRow[];
    topCountries: ApiReportRow[];
    topFlows: ApiReportRow[];
    topOffers: ApiReportRow[];
  }>("/analytics/dashboard", {
    method: "POST",
    body: { from: formatDate(from), to: formatDate(to), timezone, dateBasis },
  }).then((res) => ({
    totals: res.totals,
    datedByConversion: res.datedByConversion ?? 0,
    daily: res.daily.map((d) => ({ day: d.day!, metrics: d.metrics })),
    topCampaigns: res.topCampaigns.map((r) => toReportRow(r, ["campaign"])),
    topCountries: res.topCountries.map((r) => toReportRow(r, ["country"])),
    topFlows: res.topFlows.map((r) => toReportRow(r, ["flow"])),
    topOffers: res.topOffers.map((r) => toReportRow(r, ["offer"])),
  }));
}

/**
 * The ad-account time zones whose daily spend in the range is cut in other
 * days than the report's (Go analytics.CostTimezones, tech audit AN4) —
 * empty when they all match or there is no synced spend.
 */
export function getCostTimezones(from: Date, to: Date, timezone?: string): Promise<string[]> {
  return apiFetch<{ mismatched: string[] }>("/analytics/cost-timezones", {
    method: "POST",
    body: { from: formatDate(from), to: formatDate(to), timezone },
  }).then((res) => res.mismatched);
}

/** One flow's per-stage counts, keyed by event type — every stage the
 * backend counts is present (zero-filled). See
 * apps/internal/chstore.FlowStageCounts: distinct visits for clicks and the
 * landing/PWA/postlanding stages, events for CPA_ACCEPT/CPA_REDEP. */
export type FlowStageCounts = Record<string, number>;

/** Per-flow stage counts for the flow builder's nodes (§25 "analytics
 * summary"). Another organization's flow id simply comes back as zeros. */
export function getFlowSummary(flowIds: string[], from: Date, to: Date, timezone?: string): Promise<Record<string, FlowStageCounts>> {
  return apiFetch<{ flows: { flowId: string; counts: FlowStageCounts }[] }>("/analytics/flow-summary", {
    method: "POST",
    body: { flowIds, from: formatDate(from), to: formatDate(to), timezone },
  }).then((res) => Object.fromEntries(res.flows.map((f) => [f.flowId, f.counts])));
}

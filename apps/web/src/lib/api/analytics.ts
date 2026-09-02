import { apiFetch } from "@/lib/api/client";
import type { DimensionKey, MetricKey } from "@/features/analytics/registry";
import type { FilterCondition, ReportRow } from "@/features/analytics/aggregate";

/** Mirrors apps/internal/analytics's response shapes exactly. */
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
): Promise<{ revenue: DailyRevenue[] }> {
  return apiFetch(`/analytics/campaigns/${campaignId}/daily-revenue`, {
    searchParams: { from: formatDate(from), to: formatDate(to) },
  });
}

// --- Phase 35: Report/Timeseries/Funnel/Dashboard ---
// Mirrors apps/internal/analytics/handler.go's metricValuesResponse/
// reportRowResponse exactly — one metric-values shape reused by every
// endpoint below, same as the Go side.

type ApiMetrics = Record<MetricKey, number | null>;

type ApiReportRow = { day?: string; dims?: Record<string, string>; metrics: ApiMetrics };

function toReportRow(row: ApiReportRow, dims: DimensionKey[]): ReportRow {
  const dimValues = (row.dims ?? {}) as Partial<Record<DimensionKey, string>>;
  const key = row.day ?? (dims.length === 0 ? "__all__" : dims.map((d) => dimValues[d] ?? "").join(" · "));
  return { key, dims: dimValues, metrics: row.metrics };
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
}): Promise<ReportRow[]> {
  return apiFetch<{ rows: ApiReportRow[] }>("/analytics/report", {
    method: "POST",
    body: {
      dims: input.dims,
      filters: toFilterBody(input.filters),
      from: formatDate(input.from),
      to: formatDate(input.to),
      sort: input.sort,
    },
  }).then((res) => res.rows.map((r) => toReportRow(r, input.dims)));
}

export function getTimeseries(input: {
  filters: FilterCondition[];
  from: Date;
  to: Date;
}): Promise<TimeseriesPoint[]> {
  return apiFetch<{ rows: ApiReportRow[] }>("/analytics/timeseries", {
    method: "POST",
    body: {
      filters: toFilterBody(input.filters),
      from: formatDate(input.from),
      to: formatDate(input.to),
    },
  }).then((res) => res.rows.map((r) => ({ day: r.day!, metrics: r.metrics })));
}

export type FunnelStepData = { step: string; value: number };

export function getFunnel(from: Date, to: Date): Promise<FunnelStepData[]> {
  return apiFetch<{ steps: FunnelStepData[] }>("/analytics/funnel", {
    method: "POST",
    body: { from: formatDate(from), to: formatDate(to) },
  }).then((res) => res.steps);
}

export type DashboardReport = {
  totals: ApiMetrics;
  daily: TimeseriesPoint[];
  topCampaigns: ReportRow[];
  topCountries: ReportRow[];
  topFlows: ReportRow[];
};

export function getDashboardReport(from: Date, to: Date): Promise<DashboardReport> {
  return apiFetch<{
    totals: ApiMetrics;
    daily: ApiReportRow[];
    topCampaigns: ApiReportRow[];
    topCountries: ApiReportRow[];
    topFlows: ApiReportRow[];
  }>("/analytics/dashboard", {
    method: "POST",
    body: { from: formatDate(from), to: formatDate(to) },
  }).then((res) => ({
    totals: res.totals,
    daily: res.daily.map((d) => ({ day: d.day!, metrics: d.metrics })),
    topCampaigns: res.topCampaigns.map((r) => toReportRow(r, ["campaign"])),
    topCountries: res.topCountries.map((r) => toReportRow(r, ["country"])),
    topFlows: res.topFlows.map((r) => toReportRow(r, ["flow"])),
  }));
}

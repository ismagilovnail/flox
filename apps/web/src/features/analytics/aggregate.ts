import type { DateRange } from "react-day-picker";

import type { RawSlice } from "@/lib/mock/analytics";
import type { DimensionKey, MetricKey } from "@/features/analytics/registry";

export type FilterCondition = { dimension: DimensionKey; value: string };

export type ReportRow = {
  key: string;
  dims: Partial<Record<DimensionKey, string>>;
  metrics: Record<MetricKey, number | null>;
};

function toDate(iso: string) {
  return new Date(`${iso}T00:00:00Z`);
}

function matchesFilters(slice: RawSlice, filters: FilterCondition[]) {
  return filters.every((f) => slice[f.dimension] === f.value);
}

function filterSlices(slices: RawSlice[], dateRange: DateRange | undefined, filters: FilterCondition[]) {
  const from = dateRange?.from;
  const to = dateRange?.to ?? dateRange?.from;
  return slices.filter((s) => {
    if (from && to) {
      const d = toDate(s.date);
      if (d < from || d > to) return false;
    }
    return matchesFilters(s, filters);
  });
}

function deriveMetrics(sums: {
  clicks: number;
  uniqueClicks: number;
  landClicks: number;
  conversions: number;
  revenue: number;
  cost: number;
  hasAnyCost: boolean;
}): Record<MetricKey, number | null> {
  const { clicks, uniqueClicks, landClicks, conversions, revenue, cost, hasAnyCost } = sums;
  const profit = hasAnyCost ? revenue - cost : null;
  return {
    clicks,
    uniqueClicks,
    conversions,
    revenue,
    cost: hasAnyCost ? cost : null,
    profit,
    roi: hasAnyCost && cost > 0 ? (profit! / cost) * 100 : null,
    roas: hasAnyCost && cost > 0 ? revenue / cost : null,
    ctr: clicks > 0 ? (landClicks / clicks) * 100 : 0,
    cvr: clicks > 0 ? (conversions / clicks) * 100 : 0,
    cpc: hasAnyCost && clicks > 0 ? cost / clicks : null,
    cpa: hasAnyCost && conversions > 0 ? cost / conversions : null,
    epc: clicks > 0 ? revenue / clicks : 0,
  };
}

function sumSlices(group: RawSlice[]) {
  return group.reduce(
    (acc, s) => {
      acc.clicks += s.clicks;
      acc.uniqueClicks += s.uniqueClicks;
      acc.landClicks += s.landClicks;
      acc.conversions += s.conversions;
      acc.revenue += s.revenue;
      if (s.hasCost) {
        acc.cost += s.cost;
        acc.hasAnyCost = true;
      }
      return acc;
    },
    { clicks: 0, uniqueClicks: 0, landClicks: 0, conversions: 0, revenue: 0, cost: 0, hasAnyCost: false },
  );
}

export function aggregateReport({
  slices,
  dateRange,
  dimensions,
  filters,
  sort,
}: {
  slices: RawSlice[];
  dateRange?: DateRange;
  dimensions: DimensionKey[];
  filters: FilterCondition[];
  sort: { key: string; dir: "asc" | "desc" };
}): ReportRow[] {
  const filtered = filterSlices(slices, dateRange, filters);

  const groups = new Map<string, RawSlice[]>();
  for (const slice of filtered) {
    const key =
      dimensions.length === 0
        ? "__all__"
        : dimensions.map((d) => slice[d]).join(" · ");
    const group = groups.get(key);
    if (group) group.push(slice);
    else groups.set(key, [slice]);
  }

  const rows: ReportRow[] = Array.from(groups.entries()).map(([key, group]) => {
    const dims: Partial<Record<DimensionKey, string>> =
      dimensions.length === 0
        ? {}
        : Object.fromEntries(dimensions.map((d) => [d, group[0][d]]));

    return { key, dims, metrics: deriveMetrics(sumSlices(group)) };
  });

  const dir = sort.dir === "asc" ? 1 : -1;
  rows.sort((a, b) => {
    const av = (a.dims[sort.key as DimensionKey] ?? a.metrics[sort.key as MetricKey]) ?? -Infinity;
    const bv = (b.dims[sort.key as DimensionKey] ?? b.metrics[sort.key as MetricKey]) ?? -Infinity;
    if (typeof av === "string" || typeof bv === "string") {
      return dir * String(av).localeCompare(String(bv));
    }
    return dir * ((av as number) - (bv as number));
  });

  return rows;
}

/** Daily series for the selected metric, ignoring the dimension breakdown — feeds the line chart. */
export function aggregateTimeSeries({
  slices,
  dateRange,
  filters,
  metric,
}: {
  slices: RawSlice[];
  dateRange?: DateRange;
  filters: FilterCondition[];
  metric: MetricKey;
}): { date: string; value: number | null }[] {
  const filtered = filterSlices(slices, dateRange, filters);
  const byDate = new Map<string, RawSlice[]>();
  for (const slice of filtered) {
    const group = byDate.get(slice.date);
    if (group) group.push(slice);
    else byDate.set(slice.date, [slice]);
  }
  return Array.from(byDate.entries())
    .map(([date, group]) => ({ date, value: deriveMetrics(sumSlices(group))[metric] }))
    .sort((a, b) => a.date.localeCompare(b.date));
}

/** Top-N breakdown by a single dimension, independent of the table's multi-dimension grouping — feeds the bar chart. */
export function aggregateByDimension({
  slices,
  dateRange,
  filters,
  dimension,
  metric,
  limit = 8,
}: {
  slices: RawSlice[];
  dateRange?: DateRange;
  filters: FilterCondition[];
  dimension: DimensionKey;
  metric: MetricKey;
  limit?: number;
}): { name: string; value: number | null }[] {
  const filtered = filterSlices(slices, dateRange, filters);
  const byValue = new Map<string, RawSlice[]>();
  for (const slice of filtered) {
    const key = slice[dimension];
    const group = byValue.get(key);
    if (group) group.push(slice);
    else byValue.set(key, [slice]);
  }
  return Array.from(byValue.entries())
    .map(([name, group]) => ({ name, value: deriveMetrics(sumSlices(group))[metric] }))
    .sort((a, b) => (b.value ?? -Infinity) - (a.value ?? -Infinity))
    .slice(0, limit);
}

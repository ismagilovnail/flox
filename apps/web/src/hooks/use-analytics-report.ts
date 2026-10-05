"use client";

import { useQuery } from "@tanstack/react-query";

import { getReport, getTimeseries, getFunnel, getDashboardReport, getDimensionValues } from "@/lib/api/analytics";
import type { DimensionKey } from "@/features/analytics/registry";
import type { FilterCondition } from "@/features/analytics/aggregate";

function iso(d: Date): string {
  return d.toISOString().slice(0, 10);
}

export function useReport(
  input: {
    dims: DimensionKey[];
    filters: FilterCondition[];
    from: Date;
    to: Date;
    sort: { key: string; dir: "asc" | "desc" };
  },
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: ["analytics-report", input.dims, input.filters, iso(input.from), iso(input.to), input.sort],
    queryFn: () => getReport(input),
    enabled: options?.enabled,
  });
}

export function useTimeseries(input: { filters: FilterCondition[]; from: Date; to: Date }) {
  return useQuery({
    queryKey: ["analytics-timeseries", input.filters, iso(input.from), iso(input.to)],
    queryFn: () => getTimeseries(input),
  });
}

export function useFunnelReport(from: Date, to: Date) {
  return useQuery({
    queryKey: ["analytics-funnel", iso(from), iso(to)],
    queryFn: () => getFunnel(from, to),
  });
}

export function useDashboardReport(from: Date, to: Date) {
  return useQuery({
    queryKey: ["dashboard-report", iso(from), iso(to)],
    queryFn: () => getDashboardReport(from, to),
  });
}

/**
 * `retry: false` — a rejection here is a deterministic 400 for one of
 * the 2 still-unbacked dimensions (chstore.ValidDimension), not a
 * transient failure retrying would ever fix; the caller (report-
 * controls.tsx) falls back to the mock pool on `isError`, so retrying
 * would only delay that fallback for no benefit. `enabled` lets the
 * caller skip firing this until its popover is actually open.
 */
export function useDimensionValues(dim: DimensionKey, from: Date, to: Date, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ["analytics-dimension-values", dim, iso(from), iso(to)],
    queryFn: () => getDimensionValues(dim, from, to),
    retry: false,
    enabled: options?.enabled,
  });
}

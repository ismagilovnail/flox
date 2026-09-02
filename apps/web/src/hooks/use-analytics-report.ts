"use client";

import { useQuery } from "@tanstack/react-query";

import { getReport, getTimeseries, getFunnel, getDashboardReport } from "@/lib/api/analytics";
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

"use client";

import { useQuery } from "@tanstack/react-query";

import {
  getReport,
  getTimeseries,
  getFunnel,
  getDashboardReport,
  getDimensionValues,
  getCostTimezones,
  getFlowSummary,
  type DateBasis,
} from "@/lib/api/analytics";
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
    timezone?: string;
    dateBasis?: DateBasis;
  },
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: [
      "analytics-report",
      input.dims,
      input.filters,
      iso(input.from),
      iso(input.to),
      input.sort,
      input.timezone,
      input.dateBasis,
    ],
    queryFn: () => getReport(input),
    enabled: options?.enabled,
  });
}

export function useTimeseries(input: {
  filters: FilterCondition[];
  from: Date;
  to: Date;
  timezone?: string;
  dateBasis?: DateBasis;
}) {
  return useQuery({
    queryKey: ["analytics-timeseries", input.filters, iso(input.from), iso(input.to), input.timezone, input.dateBasis],
    queryFn: () => getTimeseries(input),
  });
}

export function useFunnelReport(from: Date, to: Date, filters: FilterCondition[] = [], timezone?: string) {
  return useQuery({
    queryKey: ["analytics-funnel", iso(from), iso(to), filters, timezone],
    queryFn: () => getFunnel(from, to, filters, timezone),
  });
}

export function useDashboardReport(from: Date, to: Date, timezone?: string, dateBasis: DateBasis = "conversion") {
  return useQuery({
    queryKey: ["dashboard-report", iso(from), iso(to), timezone, dateBasis],
    queryFn: () => getDashboardReport(from, to, timezone, dateBasis),
  });
}

/**
 * `retry: false` — a rejection here is a deterministic 400 for a
 * dimension outside chstore.ValidDimension, not a transient failure
 * retrying would ever fix; the report builder offers only valid ones, so
 * in practice it never fires. `enabled` lets the caller skip firing this
 * until its popover is actually open.
 */
export function useDimensionValues(dim: DimensionKey, from: Date, to: Date, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ["analytics-dimension-values", dim, iso(from), iso(to)],
    queryFn: () => getDimensionValues(dim, from, to),
    retry: false,
    enabled: options?.enabled,
  });
}

/** Ad-account zones that cut spend into other days than the report (AN4). */
export function useCostTimezones(from: Date, to: Date, timezone?: string) {
  return useQuery({
    queryKey: ["analytics-cost-timezones", iso(from), iso(to), timezone],
    queryFn: () => getCostTimezones(from, to, timezone),
  });
}

/** Stage counts for a stream set's saved flows (§25 node "analytics
 * summary"). `enabled` waits for the caller's timezone. */
export function useFlowSummary(flowIds: string[], from: Date, to: Date, timezone: string | undefined, enabled = true) {
  return useQuery({
    queryKey: ["analytics-flow-summary", flowIds, iso(from), iso(to), timezone],
    queryFn: () => getFlowSummary(flowIds, from, to, timezone),
    enabled: enabled && flowIds.length > 0,
  });
}

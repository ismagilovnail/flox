import type { DimensionKey, MetricKey } from "@/features/analytics/registry";

/**
 * Client-side aggregation over mock data was removed once /analytics
 * (Phase 35, `apps/internal/analytics/report.go`) started answering these
 * shapes for real — see `@/lib/api/analytics`'s `getReport`/`getTimeseries`/
 * `getFunnel`/`getDashboardReport`, which return exactly the two types
 * below (mapped from the API response) instead of re-deriving them from
 * raw slices. Kept here since `ReportTable` (and its custom-metric formula
 * evaluation) and the report-builder screens still key off these shapes.
 */

export type FilterCondition = { dimension: DimensionKey; value: string };

export type ReportRow = {
  key: string;
  dims: Partial<Record<DimensionKey, string>>;
  metrics: Record<MetricKey, number | null>;
};

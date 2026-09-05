import type { MetricFormat } from "@/features/analytics/registry";
import type { ShowInTarget } from "@/lib/custom-metrics-registry";

/** Real domain type (§30.5) — mirrors apps/internal/custommetrics.CustomMetric's
 * JSON shape exactly. Not mock data, so it doesn't live under lib/mock/. */
export type CustomMetricStatus = "draft" | "published";

export type CustomMetric = {
  id: string;
  organizationId: string;
  name: string;
  group: string;
  formula: string;
  format: MetricFormat;
  status: CustomMetricStatus;
  active: boolean;
  targets: ShowInTarget[];
  createdByUserId: string;
  createdAt: string;
  updatedAt: string;
};

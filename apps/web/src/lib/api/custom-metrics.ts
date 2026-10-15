import { apiFetch } from "@/lib/api/client";
import type { CustomMetric, CustomMetricStatus } from "@/lib/custom-metrics";
import type { ShowInTarget } from "@/lib/custom-metrics-registry";
import type { MetricFormat } from "@/types/analytics";

export type CustomMetricInput = {
  name: string;
  group: string;
  formula: string;
  format: MetricFormat;
  targets: ShowInTarget[];
  status: CustomMetricStatus;
};

export function listCustomMetrics(): Promise<{ metrics: CustomMetric[] }> {
  return apiFetch("/custom-metrics");
}

export function getCustomMetric(id: string): Promise<CustomMetric> {
  return apiFetch(`/custom-metrics/${id}`);
}

export function createCustomMetric(input: CustomMetricInput): Promise<CustomMetric> {
  return apiFetch("/custom-metrics", { method: "POST", body: input });
}

export function updateCustomMetric(id: string, input: Partial<CustomMetricInput>): Promise<CustomMetric> {
  return apiFetch(`/custom-metrics/${id}`, { method: "PATCH", body: input });
}

export function deleteCustomMetric(id: string): Promise<void> {
  return apiFetch(`/custom-metrics/${id}`, { method: "DELETE" });
}

export function duplicateCustomMetric(id: string): Promise<CustomMetric> {
  return apiFetch(`/custom-metrics/${id}/duplicate`, { method: "POST" });
}

export function setCustomMetricActive(id: string, active: boolean): Promise<CustomMetric> {
  return apiFetch(`/custom-metrics/${id}`, { method: "PATCH", body: { active } });
}

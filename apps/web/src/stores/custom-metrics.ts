import { create } from "zustand";

import { genId } from "@/lib/id";
import { CUSTOM_METRICS, type CustomMetric, type CustomMetricStatus } from "@/lib/mock/custom-metrics";
import type { ShowInTarget } from "@/lib/mock/custom-metrics-registry";

export type CustomMetricInput = {
  name: string;
  group: string;
  formula: string;
  format: CustomMetric["format"];
  targets: ShowInTarget[];
  status: CustomMetricStatus;
};

type CustomMetricsState = {
  metrics: CustomMetric[];
  getById: (id: string) => CustomMetric | undefined;
  addMetric: (input: CustomMetricInput, createdByMemberId: string) => string;
  updateMetric: (id: string, input: CustomMetricInput) => void;
  setActive: (id: string, active: boolean) => void;
  /** Deletion blocked while published or exposed on any surface — archive
   * (deactivate) instead. Returns false and does nothing if blocked. */
  deleteMetric: (id: string) => boolean;
};

export const useCustomMetricsStore = create<CustomMetricsState>()((set, get) => ({
  metrics: [...CUSTOM_METRICS],

  getById: (id) => get().metrics.find((m) => m.id === id),

  addMetric: (input, createdByMemberId) => {
    const id = genId();
    const now = new Date().toISOString();
    const metric: CustomMetric = {
      id,
      ...input,
      active: input.status === "published",
      createdByMemberId,
      createdAt: now,
      updatedAt: now,
    };
    set((s) => ({ metrics: [metric, ...s.metrics] }));
    return id;
  },

  updateMetric: (id, input) => {
    set((s) => ({
      metrics: s.metrics.map((m) => (m.id === id ? { ...m, ...input, updatedAt: new Date().toISOString() } : m)),
    }));
  },

  setActive: (id, active) => {
    set((s) => ({
      metrics: s.metrics.map((m) => (m.id === id ? { ...m, active, updatedAt: new Date().toISOString() } : m)),
    }));
  },

  deleteMetric: (id) => {
    const metric = get().metrics.find((m) => m.id === id);
    if (!metric) return false;
    if (metric.status === "published" || metric.targets.length > 0) return false;
    set((s) => ({ metrics: s.metrics.filter((m) => m.id !== id) }));
    return true;
  },
}));

import { create } from "zustand";

import { genId } from "@/lib/id";
import { REPORT_PRESETS, type ReportPeriod, type ReportPreset } from "@/lib/mock/report-presets";
import type { DimensionKey, MetricKey } from "@/features/analytics/registry";

export type ReportPresetInput = {
  name: string;
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  groupBy: DimensionKey;
  period: ReportPeriod;
  timezone: string;
};

type ReportPresetsState = {
  presets: ReportPreset[];
  getById: (id: string) => ReportPreset | undefined;
  createPreset: (input: ReportPresetInput) => string;
  /** Also used for "edit" — a preset's config is just resaved under the same id. */
  updatePreset: (id: string, input: ReportPresetInput) => boolean;
  deletePreset: (id: string) => boolean;
};

export const useReportPresetsStore = create<ReportPresetsState>()((set, get) => ({
  presets: [...REPORT_PRESETS],

  getById: (id) => get().presets.find((p) => p.id === id),

  createPreset: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const preset: ReportPreset = { id, ...input, isDefault: false, createdAt: now, updatedAt: now };
    set((s) => ({ presets: [...s.presets, preset] }));
    return id;
  },

  updatePreset: (id, input) => {
    const target = get().presets.find((p) => p.id === id);
    if (!target || target.isDefault) return false;
    set((s) => ({
      presets: s.presets.map((p) => (p.id === id ? { ...p, ...input, updatedAt: new Date().toISOString() } : p)),
    }));
    return true;
  },

  deletePreset: (id) => {
    const target = get().presets.find((p) => p.id === id);
    if (!target || target.isDefault) return false;
    set((s) => ({ presets: s.presets.filter((p) => p.id !== id) }));
    return true;
  },
}));

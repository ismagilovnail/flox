import { create } from "zustand";

import { genId } from "@/lib/id";
import {
  TRAFFIC_SOURCES,
  type TrafficSource,
  type SourceStatus,
} from "@/lib/mock/traffic-sources";

export type TrafficSourceInput = {
  name: string;
  type: TrafficSource["type"];
  trackingTemplate: string;
  costIntegration: TrafficSource["costIntegration"];
  status: SourceStatus;
};

type TrafficSourcesState = {
  sources: TrafficSource[];
  getById: (id: string) => TrafficSource | undefined;
  addSource: (input: TrafficSourceInput) => string;
  updateSource: (id: string, input: TrafficSourceInput) => void;
  setStatus: (id: string, status: SourceStatus) => void;
  duplicateSource: (id: string) => string | undefined;
};

export const useTrafficSourcesStore = create<TrafficSourcesState>()((set, get) => ({
  sources: [...TRAFFIC_SOURCES],

  getById: (id) => get().sources.find((s) => s.id === id),

  addSource: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const source: TrafficSource = { id, ...input, createdAt: now, updatedAt: now };
    set((s) => ({ sources: [source, ...s.sources] }));
    return id;
  },

  updateSource: (id, input) => {
    set((s) => ({
      sources: s.sources.map((src) => (src.id === id ? { ...src, ...input, updatedAt: new Date().toISOString() } : src)),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      sources: s.sources.map((src) => (src.id === id ? { ...src, status, updatedAt: new Date().toISOString() } : src)),
    }));
  },

  duplicateSource: (id) => {
    const source = get().sources.find((s) => s.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: TrafficSource = { ...source, id: newId, name: `${source.name} (Copy)`, createdAt: now, updatedAt: now };
    set((s) => ({ sources: [copy, ...s.sources] }));
    return newId;
  },
}));

import { create } from "zustand";

import {
  genId,
  generateStreamSets,
  type StreamSet,
  type StreamSetStatus,
} from "@/lib/mock/stream-sets";

export type StreamSetInput = {
  name: string;
  status: StreamSetStatus;
  joiner: "AND" | "OR";
  filters: StreamSet["filters"];
  flows: StreamSet["flows"];
  pixels: string[];
  fallbackUrl: string;
};

/**
 * Generated data is cached outside the store (keyed by campaign id) so a read
 * before first mutation returns a referentially stable array. Generating a
 * fresh array inside the `listByCampaign` selector itself — even without a
 * `set()` call — breaks `useSyncExternalStore`'s snapshot-stability check and
 * causes an infinite render loop, since every render would see the selector
 * return a "changed" (new-reference) snapshot.
 */
const generatedCache = new Map<string, StreamSet[]>();

function getGenerated(campaignId: string): StreamSet[] {
  let cached = generatedCache.get(campaignId);
  if (!cached) {
    cached = generateStreamSets(campaignId);
    generatedCache.set(campaignId, cached);
  }
  return cached;
}

type StreamSetsState = {
  byCampaign: Record<string, StreamSet[]>;
  listByCampaign: (campaignId: string) => StreamSet[];
  addStreamSet: (campaignId: string, input: StreamSetInput) => string;
  updateStreamSet: (campaignId: string, id: string, input: StreamSetInput) => void;
  setStatus: (campaignId: string, id: string, status: StreamSetStatus) => void;
  duplicateStreamSet: (campaignId: string, id: string) => string | undefined;
  reorder: (campaignId: string, orderedIds: string[]) => void;
};

export const useStreamSetsStore = create<StreamSetsState>()((set, get) => ({
  byCampaign: {},

  listByCampaign: (campaignId) => get().byCampaign[campaignId] ?? getGenerated(campaignId),

  addStreamSet: (campaignId, input) => {
    const id = genId();
    const now = new Date().toISOString();
    const list = get().byCampaign[campaignId] ?? getGenerated(campaignId);
    const streamSet: StreamSet = {
      id,
      campaignId,
      priority: list.length + 1,
      createdAt: now,
      updatedAt: now,
      ...input,
    };
    set((s) => ({
      byCampaign: { ...s.byCampaign, [campaignId]: [...list, streamSet] },
    }));
    return id;
  },

  updateStreamSet: (campaignId, id, input) => {
    const list = get().byCampaign[campaignId] ?? getGenerated(campaignId);
    set((s) => ({
      byCampaign: {
        ...s.byCampaign,
        [campaignId]: list.map((item) =>
          item.id === id ? { ...item, ...input, updatedAt: new Date().toISOString() } : item,
        ),
      },
    }));
  },

  setStatus: (campaignId, id, status) => {
    const list = get().byCampaign[campaignId] ?? getGenerated(campaignId);
    set((s) => ({
      byCampaign: {
        ...s.byCampaign,
        [campaignId]: list.map((item) =>
          item.id === id ? { ...item, status, updatedAt: new Date().toISOString() } : item,
        ),
      },
    }));
  },

  duplicateStreamSet: (campaignId, id) => {
    const list = get().byCampaign[campaignId] ?? getGenerated(campaignId);
    const source = list.find((s) => s.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: StreamSet = {
      ...source,
      id: newId,
      name: `${source.name} (Copy)`,
      priority: list.length + 1,
      filters: source.filters.map((f) => ({ ...f, id: genId() })),
      flows: source.flows.map((f) => ({ ...f, id: genId() })),
      createdAt: now,
      updatedAt: now,
    };
    set((s) => ({
      byCampaign: { ...s.byCampaign, [campaignId]: [...list, copy] },
    }));
    return newId;
  },

  reorder: (campaignId, orderedIds) => {
    const list = get().byCampaign[campaignId] ?? getGenerated(campaignId);
    const byId = new Map(list.map((s) => [s.id, s]));
    const reordered = orderedIds
      .map((id, i) => {
        const streamSet = byId.get(id);
        return streamSet ? { ...streamSet, priority: i + 1 } : undefined;
      })
      .filter((s): s is StreamSet => s !== undefined);
    set((s) => ({ byCampaign: { ...s.byCampaign, [campaignId]: reordered } }));
  },
}));

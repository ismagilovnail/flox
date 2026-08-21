import { create } from "zustand";

import { genId } from "@/lib/id";
import { EVENT_MAPPINGS, type EventMapping } from "@/lib/mock/event-mappings";
import type { CpaStatus } from "@/lib/api/conversions";

export type EventMappingInput = {
  networkId: string;
  networkStatus: string;
  floxStatus: CpaStatus;
};

type EventMappingsState = {
  mappings: EventMapping[];
  addMapping: (input: EventMappingInput) => string;
  removeMapping: (id: string) => void;
};

export const useEventMappingsStore = create<EventMappingsState>()((set) => ({
  mappings: [...EVENT_MAPPINGS],

  addMapping: (input) => {
    const id = genId();
    set((s) => ({ mappings: [...s.mappings, { id, ...input }] }));
    return id;
  },

  removeMapping: (id) => {
    set((s) => ({ mappings: s.mappings.filter((m) => m.id !== id) }));
  },
}));

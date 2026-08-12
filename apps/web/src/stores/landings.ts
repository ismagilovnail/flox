import { create } from "zustand";

import { genId } from "@/lib/id";
import { LANDINGS, type Landing, type LandingStatus } from "@/lib/mock/landings";

export type LandingInput = {
  name: string;
  type: Landing["type"];
  url: string;
  content: string;
  status: LandingStatus;
};

type LandingsState = {
  landings: Landing[];
  getById: (id: string) => Landing | undefined;
  addLanding: (input: LandingInput) => string;
  updateLanding: (id: string, input: LandingInput) => void;
  setStatus: (id: string, status: LandingStatus) => void;
  duplicateLanding: (id: string) => string | undefined;
};

export const useLandingsStore = create<LandingsState>()((set, get) => ({
  landings: [...LANDINGS],

  getById: (id) => get().landings.find((l) => l.id === id),

  addLanding: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const landing: Landing = { id, ...input, createdAt: now, updatedAt: now };
    set((s) => ({ landings: [landing, ...s.landings] }));
    return id;
  },

  updateLanding: (id, input) => {
    set((s) => ({
      landings: s.landings.map((l) => (l.id === id ? { ...l, ...input, updatedAt: new Date().toISOString() } : l)),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      landings: s.landings.map((l) => (l.id === id ? { ...l, status, updatedAt: new Date().toISOString() } : l)),
    }));
  },

  duplicateLanding: (id) => {
    const source = get().landings.find((l) => l.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: Landing = { ...source, id: newId, name: `${source.name} (Copy)`, createdAt: now, updatedAt: now };
    set((s) => ({ landings: [copy, ...s.landings] }));
    return newId;
  },
}));

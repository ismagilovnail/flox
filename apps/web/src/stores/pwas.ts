import { create } from "zustand";

import { genId } from "@/lib/id";
import { PWAS, type Pwa, type PwaStatus } from "@/lib/mock/pwas";

export type PwaInput = {
  name: string;
  shortName: string;
  themeColor: string;
  backgroundColor: string;
  iconUrl: string;
  startUrl: string;
  bounceInAppWebview: boolean;
  status: PwaStatus;
};

type PwasState = {
  pwas: Pwa[];
  getById: (id: string) => Pwa | undefined;
  addPwa: (input: PwaInput) => string;
  updatePwa: (id: string, input: PwaInput) => void;
  setStatus: (id: string, status: PwaStatus) => void;
  duplicatePwa: (id: string) => string | undefined;
};

export const usePwasStore = create<PwasState>()((set, get) => ({
  pwas: [...PWAS],

  getById: (id) => get().pwas.find((p) => p.id === id),

  addPwa: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const pwa: Pwa = { id, ...input, createdAt: now, updatedAt: now };
    set((s) => ({ pwas: [pwa, ...s.pwas] }));
    return id;
  },

  updatePwa: (id, input) => {
    set((s) => ({
      pwas: s.pwas.map((p) => (p.id === id ? { ...p, ...input, updatedAt: new Date().toISOString() } : p)),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      pwas: s.pwas.map((p) => (p.id === id ? { ...p, status, updatedAt: new Date().toISOString() } : p)),
    }));
  },

  duplicatePwa: (id) => {
    const source = get().pwas.find((p) => p.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: Pwa = { ...source, id: newId, name: `${source.name} (Copy)`, createdAt: now, updatedAt: now };
    set((s) => ({ pwas: [copy, ...s.pwas] }));
    return newId;
  },
}));

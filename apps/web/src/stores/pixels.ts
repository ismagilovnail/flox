import { create } from "zustand";

import { genId } from "@/lib/id";
import { PIXELS, type Pixel, type PixelStatus } from "@/lib/mock/pixels";

export type PixelInput = {
  name: string;
  provider: Pixel["provider"];
  pixelId: string;
  events: Pixel["events"];
  status: PixelStatus;
};

type PixelsState = {
  pixels: Pixel[];
  getById: (id: string) => Pixel | undefined;
  addPixel: (input: PixelInput) => string;
  updatePixel: (id: string, input: PixelInput) => void;
  setStatus: (id: string, status: PixelStatus) => void;
  duplicatePixel: (id: string) => string | undefined;
};

export const usePixelsStore = create<PixelsState>()((set, get) => ({
  pixels: [...PIXELS],

  getById: (id) => get().pixels.find((p) => p.id === id),

  addPixel: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const pixel: Pixel = { id, ...input, createdAt: now, updatedAt: now };
    set((s) => ({ pixels: [pixel, ...s.pixels] }));
    return id;
  },

  updatePixel: (id, input) => {
    set((s) => ({
      pixels: s.pixels.map((p) => (p.id === id ? { ...p, ...input, updatedAt: new Date().toISOString() } : p)),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      pixels: s.pixels.map((p) => (p.id === id ? { ...p, status, updatedAt: new Date().toISOString() } : p)),
    }));
  },

  duplicatePixel: (id) => {
    const source = get().pixels.find((p) => p.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: Pixel = { ...source, id: newId, name: `${source.name} (Copy)`, createdAt: now, updatedAt: now };
    set((s) => ({ pixels: [copy, ...s.pixels] }));
    return newId;
  },
}));

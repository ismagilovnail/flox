import { create } from "zustand";

import { genId } from "@/lib/id";
import { OFFERS, type Offer, type OfferLink, type OfferStatus } from "@/lib/mock/offers";

export type OfferInput = {
  networkId: string;
  name: string;
  countries: string[];
  payout: number;
  currency: string;
  cap: number | null;
  status: OfferStatus;
  links: OfferLink[];
};

type OffersState = {
  offers: Offer[];
  getById: (id: string) => Offer | undefined;
  addOffer: (input: OfferInput) => string;
  updateOffer: (id: string, input: OfferInput) => void;
  setStatus: (id: string, status: OfferStatus) => void;
  duplicateOffer: (id: string) => string | undefined;
};

export const useOffersStore = create<OffersState>()((set, get) => ({
  offers: [...OFFERS],

  getById: (id) => get().offers.find((o) => o.id === id),

  addOffer: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const offer: Offer = { id, ...input, createdAt: now, updatedAt: now };
    set((s) => ({ offers: [offer, ...s.offers] }));
    return id;
  },

  updateOffer: (id, input) => {
    set((s) => ({
      offers: s.offers.map((o) => (o.id === id ? { ...o, ...input, updatedAt: new Date().toISOString() } : o)),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      offers: s.offers.map((o) => (o.id === id ? { ...o, status, updatedAt: new Date().toISOString() } : o)),
    }));
  },

  duplicateOffer: (id) => {
    const source = get().offers.find((o) => o.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: Offer = {
      ...source,
      id: newId,
      name: `${source.name} (Copy)`,
      links: source.links.map((l) => ({ ...l, id: genId() })),
      createdAt: now,
      updatedAt: now,
    };
    set((s) => ({ offers: [copy, ...s.offers] }));
    return newId;
  },
}));

import { create } from "zustand";

import { generateCampaigns, type Campaign, type CampaignStatus } from "@/lib/mock/campaigns";

function genId() {
  const chars = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
  let out = "";
  for (let i = 0; i < 12; i++) out += chars[Math.floor(Math.random() * chars.length)];
  return out;
}

export type NewCampaignInput = {
  name: string;
  source: string;
  trackingDomain: string;
  fallbackUrl: string;
  notes: string;
};

type CampaignsState = {
  campaigns: Campaign[];
  getById: (id: string) => Campaign | undefined;
  addCampaign: (input: NewCampaignInput) => string;
  updateCampaign: (id: string, patch: Partial<NewCampaignInput>) => void;
  setStatus: (id: string, status: CampaignStatus) => void;
  duplicateCampaign: (id: string) => string | undefined;
};

export const useCampaignsStore = create<CampaignsState>()((set, get) => ({
  campaigns: generateCampaigns(),

  getById: (id) => get().campaigns.find((c) => c.id === id),

  addCampaign: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const campaign: Campaign = {
      id,
      name: input.name,
      status: "draft",
      source: input.source,
      trackingDomain: input.trackingDomain,
      trackingId: genId().slice(0, 8),
      fallbackUrl: input.fallbackUrl,
      notes: input.notes,
      createdAt: now,
      updatedAt: now,
      clicks: 0,
      conversions: 0,
      revenue: 0,
      spend: null,
    };
    set((s) => ({ campaigns: [campaign, ...s.campaigns] }));
    return id;
  },

  updateCampaign: (id, patch) => {
    set((s) => ({
      campaigns: s.campaigns.map((c) =>
        c.id === id ? { ...c, ...patch, updatedAt: new Date().toISOString() } : c,
      ),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      campaigns: s.campaigns.map((c) =>
        c.id === id ? { ...c, status, updatedAt: new Date().toISOString() } : c,
      ),
    }));
  },

  duplicateCampaign: (id) => {
    const source = get().campaigns.find((c) => c.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: Campaign = {
      ...source,
      id: newId,
      name: `${source.name} (Copy)`,
      status: "draft",
      trackingId: genId().slice(0, 8),
      createdAt: now,
      updatedAt: now,
      clicks: 0,
      conversions: 0,
      revenue: 0,
      spend: null,
    };
    set((s) => ({ campaigns: [copy, ...s.campaigns] }));
    return newId;
  },
}));

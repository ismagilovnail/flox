import { create } from "zustand";

import { genId } from "@/lib/id";
import { NETWORKS, type Network, type NetworkStatus } from "@/lib/mock/networks";

export type NetworkInput = {
  name: string;
  postbackUrl: string;
  status: NetworkStatus;
};

type NetworksState = {
  networks: Network[];
  getById: (id: string) => Network | undefined;
  addNetwork: (input: NetworkInput) => string;
  updateNetwork: (id: string, input: NetworkInput) => void;
  setStatus: (id: string, status: NetworkStatus) => void;
  duplicateNetwork: (id: string) => string | undefined;
};

export const useNetworksStore = create<NetworksState>()((set, get) => ({
  networks: [...NETWORKS],

  getById: (id) => get().networks.find((n) => n.id === id),

  addNetwork: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const network: Network = { id, ...input, createdAt: now, updatedAt: now };
    set((s) => ({ networks: [network, ...s.networks] }));
    return id;
  },

  updateNetwork: (id, input) => {
    set((s) => ({
      networks: s.networks.map((n) => (n.id === id ? { ...n, ...input, updatedAt: new Date().toISOString() } : n)),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      networks: s.networks.map((n) => (n.id === id ? { ...n, status, updatedAt: new Date().toISOString() } : n)),
    }));
  },

  duplicateNetwork: (id) => {
    const source = get().networks.find((n) => n.id === id);
    if (!source) return undefined;
    const newId = genId();
    const now = new Date().toISOString();
    const copy: Network = { ...source, id: newId, name: `${source.name} (Copy)`, createdAt: now, updatedAt: now };
    set((s) => ({ networks: [copy, ...s.networks] }));
    return newId;
  },
}));

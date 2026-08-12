import { create } from "zustand";

import { genId } from "@/lib/id";
import { DOMAINS, type Domain, type DomainStatus } from "@/lib/mock/domains";

export type DomainInput = {
  domain: string;
  purpose: Domain["purpose"];
  registrar: Domain["registrar"];
  dnsProvider: Domain["dnsProvider"];
  status: DomainStatus;
};

type DomainsState = {
  domains: Domain[];
  getById: (id: string) => Domain | undefined;
  addDomain: (input: DomainInput) => string;
  updateDomain: (id: string, input: DomainInput) => void;
  setStatus: (id: string, status: DomainStatus) => void;
  /** Mock — automatic issuance is a real cert-provider call in Phase 27+; this just reflects intent. */
  issueSsl: (id: string) => void;
  /** Mock ownership verification for own-domain parking. */
  verify: (id: string) => void;
  removeDomain: (id: string) => void;
};

export const useDomainsStore = create<DomainsState>()((set, get) => ({
  domains: [...DOMAINS],

  getById: (id) => get().domains.find((d) => d.id === id),

  addDomain: (input) => {
    const id = genId();
    const now = new Date().toISOString();
    const domain: Domain = {
      id,
      ...input,
      ssl: "none",
      expiresAt: null,
      verifiedAt: null,
      createdAt: now,
      updatedAt: now,
    };
    set((s) => ({ domains: [domain, ...s.domains] }));
    return id;
  },

  updateDomain: (id, input) => {
    set((s) => ({
      domains: s.domains.map((d) => (d.id === id ? { ...d, ...input, updatedAt: new Date().toISOString() } : d)),
    }));
  },

  setStatus: (id, status) => {
    set((s) => ({
      domains: s.domains.map((d) => (d.id === id ? { ...d, status, updatedAt: new Date().toISOString() } : d)),
    }));
  },

  issueSsl: (id) => {
    set((s) => ({
      domains: s.domains.map((d) => (d.id === id ? { ...d, ssl: "issued", updatedAt: new Date().toISOString() } : d)),
    }));
  },

  verify: (id) => {
    const now = new Date().toISOString();
    set((s) => ({
      domains: s.domains.map((d) =>
        d.id === id ? { ...d, status: "active", verifiedAt: now, updatedAt: now } : d,
      ),
    }));
  },

  removeDomain: (id) => {
    set((s) => ({ domains: s.domains.filter((d) => d.id !== id) }));
  },
}));

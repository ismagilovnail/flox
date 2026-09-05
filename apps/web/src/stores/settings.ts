import { create } from "zustand";

import { INTEGRATIONS, type Integration, type IntegrationStatus } from "@/lib/mock/settings";

/** Integrations only now — Organization, API keys, and Security all moved
 * to real backends (hooks/use-organization.ts, hooks/use-api-keys.ts,
 * hooks/use-security.ts) and no longer read from this store. */
type SettingsState = {
  integrations: Integration[];
  setIntegrationStatus: (id: string, status: IntegrationStatus) => void;
};

export const useSettingsStore = create<SettingsState>()((set) => ({
  integrations: [...INTEGRATIONS],
  setIntegrationStatus: (id, status) => {
    set((s) => ({
      integrations: s.integrations.map((i) =>
        i.id === id ? { ...i, status, connectedAt: status === "connected" ? new Date().toISOString() : null } : i,
      ),
    }));
  },
}));

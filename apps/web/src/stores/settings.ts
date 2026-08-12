import { create } from "zustand";

import { genId } from "@/lib/id";
import {
  ORG_SETTINGS,
  API_KEYS,
  INTEGRATIONS,
  SECURITY_SETTINGS,
  type ApiKey,
  type ApiKeyScope,
  type Integration,
  type IntegrationStatus,
  type OrgSettings,
  type SecuritySettings,
} from "@/lib/mock/settings";

/** Settings is one coherent config slice for the workspace (unlike Offers/Networks,
 * which are independent entity collections used from several pages) — one store,
 * not four nearly-empty ones. */
type SettingsState = {
  org: OrgSettings;
  updateOrg: (input: OrgSettings) => void;

  apiKeys: ApiKey[];
  /** Returns the one-time full key string — only the prefix is ever persisted. */
  createApiKey: (name: string, scope: ApiKeyScope) => { id: string; fullKey: string };
  revokeApiKey: (id: string) => void;

  integrations: Integration[];
  setIntegrationStatus: (id: string, status: IntegrationStatus) => void;

  security: SecuritySettings;
  updateSecurity: (input: SecuritySettings) => void;
};

export const useSettingsStore = create<SettingsState>()((set) => ({
  org: ORG_SETTINGS,
  updateOrg: (input) => set({ org: input }),

  apiKeys: [...API_KEYS],
  createApiKey: (name, scope) => {
    const id = genId();
    const secret = genId().toLowerCase();
    const fullKey = `flx_live_${secret}`;
    const key: ApiKey = {
      id,
      name,
      prefix: `flx_live_${secret.slice(0, 4)}...`,
      scope,
      createdAt: new Date().toISOString(),
      lastUsedAt: null,
    };
    set((s) => ({ apiKeys: [key, ...s.apiKeys] }));
    return { id, fullKey };
  },
  revokeApiKey: (id) => {
    set((s) => ({ apiKeys: s.apiKeys.filter((k) => k.id !== id) }));
  },

  integrations: [...INTEGRATIONS],
  setIntegrationStatus: (id, status) => {
    set((s) => ({
      integrations: s.integrations.map((i) =>
        i.id === id ? { ...i, status, connectedAt: status === "connected" ? new Date().toISOString() : null } : i,
      ),
    }));
  },

  security: SECURITY_SETTINGS,
  updateSecurity: (input) => set({ security: input }),
}));

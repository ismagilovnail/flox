/** API keys (§30, Settings) — real domain vocabulary, mirrors
 * apps/internal/apikey's shapes exactly. Not mock data, so it doesn't
 * live under lib/mock/. */

export type ApiKeyScope = "read" | "write" | "admin";
export const API_KEY_SCOPES: ApiKeyScope[] = ["read", "write", "admin"];

export type ApiKey = {
  id: string;
  name: string;
  /** Only the prefix is ever persisted/displayed after creation — the full key is shown once. */
  prefix: string;
  scope: ApiKeyScope;
  createdAt: string;
  lastUsedAt: string | null;
};

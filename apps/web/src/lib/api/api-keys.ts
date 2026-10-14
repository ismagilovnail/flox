import { apiFetch } from "@/lib/api/client";
import type { ApiKey, ApiKeyScope } from "@/lib/api-keys";

export function listApiKeys(): Promise<{ apiKeys: ApiKey[] }> {
  return apiFetch("/api-keys");
}

/** Returns the one-time full key string alongside the stored record —
 * only the prefix is ever persisted or returned again after this call. */
export function createApiKey(input: { name: string; scope: ApiKeyScope }): Promise<{ apiKey: ApiKey; fullKey: string }> {
  return apiFetch("/api-keys", { method: "POST", body: input });
}

/** A new key with the same name and scope, its full secret shown once; the
 * rotated key keeps working for 24 hours (apps/internal/apikey.RotationGrace). */
export function rotateApiKey(id: string): Promise<{ apiKey: ApiKey; fullKey: string }> {
  return apiFetch(`/api-keys/${id}/rotate`, { method: "POST" });
}

export function revokeApiKey(id: string): Promise<void> {
  return apiFetch(`/api-keys/${id}`, { method: "DELETE" });
}

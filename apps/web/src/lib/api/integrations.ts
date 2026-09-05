import { apiFetch } from "@/lib/api/client";
import type { CredentialProvider, Integration, IntegrationsSummary } from "@/lib/integrations";

export function getIntegrations(): Promise<IntegrationsSummary> {
  return apiFetch("/integrations");
}

export function connectIntegration(provider: CredentialProvider, fields: Record<string, string>): Promise<Integration> {
  return apiFetch(`/integrations/${provider}/connect`, { method: "POST", body: fields });
}

export function verifyIntegration(provider: CredentialProvider): Promise<Integration> {
  return apiFetch(`/integrations/${provider}/verify`, { method: "POST" });
}

export function disconnectIntegration(provider: CredentialProvider): Promise<void> {
  return apiFetch(`/integrations/${provider}`, { method: "DELETE" });
}

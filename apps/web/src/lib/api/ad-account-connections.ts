import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/adaccount's real shape. accessToken is
 * deliberately never part of this type — the API never echoes it back
 * after Connect, only a masked tokenPreview (last 4 characters). See
 * docs/ad-account-connections.md. */
export type AdAccountConnection = {
  id: string;
  organizationId: string;
  trafficSourceId: string;
  adAccountId: string;
  tokenPreview: string;
  createdAt: string;
  updatedAt: string;
};

export type ConnectAdAccountInput = {
  adAccountId: string;
  accessToken: string;
};

export function getAdAccountConnection(trafficSourceId: string): Promise<AdAccountConnection> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection`);
}

export function connectAdAccount(trafficSourceId: string, input: ConnectAdAccountInput): Promise<AdAccountConnection> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection`, { method: "PATCH", body: input });
}

export function disconnectAdAccount(trafficSourceId: string): Promise<void> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection`, { method: "DELETE" });
}

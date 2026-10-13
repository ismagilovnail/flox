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

/** accessToken is facebook_ads/tiktok_ads only; developerToken/clientId/
 * clientSecret/refreshToken are google_ads only — apps/internal/
 * adaccount.Service.Connect validates the right subset is present for
 * whichever cost_integration the traffic source actually has. Google
 * Ads has no long-lived access token to accept instead (its own tokens
 * expire in ~1 hour): the sync mints a short-lived one from
 * refreshToken on every run. */
export type ConnectAdAccountInput = {
  adAccountId: string;
  accessToken?: string;
  developerToken?: string;
  clientId?: string;
  clientSecret?: string;
  refreshToken?: string;
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

/** Mirrors apps/internal/costsync.Result's JSON shape (§74/§27-COST,
 * Phase B). unmatchedExternalCampaignIds is capped server-side
 * (costsync.maxUnmatchedListed) — unmatchedExternalCampaignIdsTruncated
 * is true when more existed than the list shows. */
export type SyncResult = {
  recordsFetched: number;
  entriesWritten: number;
  /** Days an operator split by country by hand: left as they are. */
  entriesSkippedSplit: number;
  unmatchedExternalCampaignIds: string[] | null;
  unmatchedExternalCampaignIdsTruncated: boolean;
};

export function syncAdAccount(trafficSourceId: string): Promise<SyncResult> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection/sync`, { method: "POST" });
}

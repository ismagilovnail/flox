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

/** Mirrors apps/internal/offlineconv.Status: a Google Ads source's export
 * of conversions back to Google (audit UNI-2 part 2). Action ids are
 * Google Ads conversion action ids; an empty one leaves that status
 * unsent. counts: per upload status; skipped: per reason. */
export type OfflineConversions = {
  enabled: boolean;
  since: string | null;
  loginAccountId: string;
  leadActionId: string;
  purchaseActionId: string;
  repurchaseActionId: string;
  counts: Partial<Record<"pending" | "sent" | "failed" | "skipped", number>>;
  skipped: Partial<Record<OfflineSkipReason, number>>;
  lastError: string;
  lastErrorAt: string | null;
  lastSentAt: string | null;
};

export type OfflineSkipReason = "not_attributed" | "click_not_found" | "no_google_click" | "source_off" | "no_action";

export type OfflineConversionsInput = Pick<
  OfflineConversions,
  "enabled" | "loginAccountId" | "leadActionId" | "purchaseActionId" | "repurchaseActionId"
>;

export function getOfflineConversions(trafficSourceId: string): Promise<OfflineConversions> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection/offline-conversions`);
}

export function updateOfflineConversions(trafficSourceId: string, input: OfflineConversionsInput): Promise<OfflineConversions> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection/offline-conversions`, { method: "PUT", body: input });
}

/** Mirrors apps/internal/offlineconv.MetricaStatus: a source's export of
 * conversions to Yandex Metrica by yclid (audit UNI-2 part 3). Goals are
 * identifiers of the counter's JavaScript-event goals; an empty one leaves
 * that status unsent. The token never comes back, only its last 4
 * characters. processing: rows Metrica has not finished matching. */
export type MetricaConnection = {
  connected: boolean;
  tokenPreview: string;
  enabled: boolean;
  since: string | null;
  counterId: string;
  currency: string;
  leadGoal: string;
  purchaseGoal: string;
  repurchaseGoal: string;
  counts: Partial<Record<"pending" | "sent" | "failed" | "skipped", number>>;
  skipped: Partial<Record<MetricaSkipReason, number>>;
  processing: number;
  lastError: string;
  lastErrorAt: string | null;
  lastSentAt: string | null;
};

export type MetricaSkipReason = "no_yandex_click" | "visit_too_old" | "no_action" | "not_attributed" | "click_not_found" | "source_off";

/** token "": keep the stored one. */
export type MetricaInput = Pick<
  MetricaConnection,
  "enabled" | "counterId" | "currency" | "leadGoal" | "purchaseGoal" | "repurchaseGoal"
> & { token: string };

export function getMetricaConnection(trafficSourceId: string): Promise<MetricaConnection> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection/metrica`);
}

export function updateMetricaConnection(trafficSourceId: string, input: MetricaInput): Promise<MetricaConnection> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection/metrica`, { method: "PUT", body: input });
}

export function deleteMetricaConnection(trafficSourceId: string): Promise<void> {
  return apiFetch(`/traffic-sources/${trafficSourceId}/connection/metrica`, { method: "DELETE" });
}

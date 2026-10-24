"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  connectAdAccount,
  disconnectAdAccount,
  getAdAccountConnection,
  getOfflineConversions,
  syncAdAccount,
  updateOfflineConversions,
  type ConnectAdAccountInput,
  type OfflineConversionsInput,
} from "@/lib/api/ad-account-connections";
import { ApiError } from "@/lib/api/client";

const connectionKey = (trafficSourceId: string) => ["ad-account-connection", trafficSourceId] as const;

/** 404 ("no ad account connected") is a normal, common state here, not
 * an error to surface via ErrorState — callers should check
 * `query.data` (undefined when not connected) rather than
 * `query.isError`. */
export function useAdAccountConnection(trafficSourceId: string) {
  return useQuery({
    queryKey: connectionKey(trafficSourceId),
    queryFn: async () => {
      try {
        return await getAdAccountConnection(trafficSourceId);
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }
    },
    enabled: !!trafficSourceId,
  });
}

export function useConnectAdAccount(trafficSourceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ConnectAdAccountInput) => connectAdAccount(trafficSourceId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: connectionKey(trafficSourceId) }),
  });
}

export function useDisconnectAdAccount(trafficSourceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => disconnectAdAccount(trafficSourceId),
    onSuccess: () => qc.invalidateQueries({ queryKey: connectionKey(trafficSourceId) }),
  });
}

/** A sync can write cost_entries for any campaign under this traffic
 * source (matched by externalCampaignId, §74/§27-COST) — the caller
 * doesn't know which campaign IDs ahead of time, so this invalidates
 * every ["cost-entries", ...] query rather than one specific campaign's
 * (see use-cost-entries.ts's own entriesKey). Sync is a manual, low-
 * frequency action, so the broad invalidation is not a real cost. */
export function useSyncAdAccount(trafficSourceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => syncAdAccount(trafficSourceId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["cost-entries"] }),
  });
}

const offlineKey = (trafficSourceId: string) => ["offline-conversions", trafficSourceId] as const;

/** A Google Ads source's export of conversions to Google (audit UNI-2
 * part 2); enabled false skips the request (no Google connection). */
export function useOfflineConversions(trafficSourceId: string, enabled: boolean) {
  return useQuery({
    queryKey: offlineKey(trafficSourceId),
    queryFn: () => getOfflineConversions(trafficSourceId),
    enabled: !!trafficSourceId && enabled,
  });
}

export function useUpdateOfflineConversions(trafficSourceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: OfflineConversionsInput) => updateOfflineConversions(trafficSourceId, input),
    onSuccess: (data) => qc.setQueryData(offlineKey(trafficSourceId), data),
  });
}

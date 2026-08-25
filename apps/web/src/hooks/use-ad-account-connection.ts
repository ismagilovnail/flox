"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  connectAdAccount,
  disconnectAdAccount,
  getAdAccountConnection,
  syncAdAccount,
  type ConnectAdAccountInput,
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

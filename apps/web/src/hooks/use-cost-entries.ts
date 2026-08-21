"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  deleteCostEntry,
  getCampaignDailySpend,
  listCostEntries,
  upsertCostEntry,
  type UpsertCostEntryInput,
} from "@/lib/api/cost-entries";

/** The last 30 days — matches useCampaignDailyClicks/Revenue's own fixed
 * window (hooks/use-campaign-analytics.ts), so Spend lines up with the
 * Revenue it's compared against. */
function last30Days() {
  const to = new Date();
  const from = new Date(to);
  from.setUTCDate(from.getUTCDate() - 29);
  return { from, to };
}

const entriesKey = (campaignId: string) => ["cost-entries", campaignId] as const;
const dailySpendKey = (campaignId: string) => ["cost-entries", campaignId, "daily"] as const;

export function useCostEntries(campaignId: string) {
  const { from, to } = last30Days();
  return useQuery({
    queryKey: entriesKey(campaignId),
    queryFn: () => listCostEntries(campaignId, from, to),
    enabled: !!campaignId,
  });
}

export function useCampaignDailySpend(campaignId: string) {
  const { from, to } = last30Days();
  return useQuery({
    queryKey: dailySpendKey(campaignId),
    queryFn: () => getCampaignDailySpend(campaignId, from, to),
    enabled: !!campaignId,
  });
}

export function useUpsertCostEntry(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpsertCostEntryInput) => upsertCostEntry(campaignId, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: entriesKey(campaignId) });
      qc.invalidateQueries({ queryKey: dailySpendKey(campaignId) });
    },
  });
}

export function useDeleteCostEntry(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteCostEntry(campaignId, id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: entriesKey(campaignId) });
      qc.invalidateQueries({ queryKey: dailySpendKey(campaignId) });
    },
  });
}

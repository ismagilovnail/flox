"use client";

import { useQuery } from "@tanstack/react-query";

import { getCampaignDaily, getCampaignDailyRevenue } from "@/lib/api/analytics";

/** The last 30 days — the campaign detail overview's own fixed window,
 * matching the mock chart it replaces ("Revenue (last 30 days)"). */
function last30Days() {
  const to = new Date();
  const from = new Date(to);
  from.setUTCDate(from.getUTCDate() - 29);
  return { from, to };
}

export function useCampaignDailyClicks(campaignId: string) {
  const { from, to } = last30Days();
  return useQuery({
    queryKey: ["campaign-daily-clicks", campaignId],
    queryFn: () => getCampaignDaily(campaignId, from, to),
    enabled: !!campaignId,
  });
}

export function useCampaignDailyRevenue(campaignId: string) {
  const { from, to } = last30Days();
  return useQuery({
    queryKey: ["campaign-daily-revenue", campaignId],
    queryFn: () => getCampaignDailyRevenue(campaignId, from, to),
    enabled: !!campaignId,
  });
}

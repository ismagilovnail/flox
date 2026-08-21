import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/analytics's response shapes exactly. */
export type DailyCount = { day: string; type: string; eventCount: number };
export type DailyRevenue = { day: string; type: string; eventCount: number; revenueUsd: number };

function formatDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

export function getCampaignDaily(campaignId: string, from: Date, to: Date): Promise<{ counts: DailyCount[] }> {
  return apiFetch(`/analytics/campaigns/${campaignId}/daily`, {
    searchParams: { from: formatDate(from), to: formatDate(to) },
  });
}

export function getCampaignDailyRevenue(
  campaignId: string,
  from: Date,
  to: Date,
): Promise<{ revenue: DailyRevenue[] }> {
  return apiFetch(`/analytics/campaigns/${campaignId}/daily-revenue`, {
    searchParams: { from: formatDate(from), to: formatDate(to) },
  });
}

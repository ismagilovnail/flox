import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/cost's response shapes exactly. */
export type CostEntry = {
  id: string;
  campaignId: string;
  trafficSourceId: string | null;
  entryDate: string;
  amount: number;
  currency: string;
  amountUsd: number | null;
  createdAt: string;
  updatedAt: string;
};

export type DailySpend = { day: string; amountUsd: number; allConverted: boolean };

export type UpsertCostEntryInput = {
  trafficSourceId?: string | null;
  entryDate: string;
  amount: number;
  currency: string;
};

function formatDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

export function listCostEntries(campaignId: string, from: Date, to: Date): Promise<{ entries: CostEntry[] }> {
  return apiFetch(`/campaigns/${campaignId}/cost-entries`, {
    searchParams: { from: formatDate(from), to: formatDate(to) },
  });
}

export function upsertCostEntry(campaignId: string, input: UpsertCostEntryInput): Promise<CostEntry> {
  return apiFetch(`/campaigns/${campaignId}/cost-entries`, { method: "POST", body: input });
}

export function deleteCostEntry(campaignId: string, id: string): Promise<void> {
  return apiFetch(`/campaigns/${campaignId}/cost-entries/${id}`, { method: "DELETE" });
}

export function getCampaignDailySpend(campaignId: string, from: Date, to: Date): Promise<{ spend: DailySpend[] }> {
  return apiFetch(`/campaigns/${campaignId}/cost-entries/daily`, {
    searchParams: { from: formatDate(from), to: formatDate(to) },
  });
}

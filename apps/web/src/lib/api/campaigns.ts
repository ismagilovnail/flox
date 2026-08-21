import { apiFetch } from "@/lib/api/client";

export type CampaignStatus = "active" | "paused" | "draft" | "archived";

/** Mirrors apps/internal/campaign.Campaign's JSON shape exactly (id,
 * organizationId, trafficSourceId, name, status, fallbackUrl, notes,
 * createdAt, updatedAt) — no trackingDomain/trackingId (that's
 * tracking_links, a separate, not-yet-built entity, see
 * docs/frontend-integration.md) and no clicks/conversions/revenue/spend
 * (those need a bulk analytics rollup that doesn't exist yet either). */
export type Campaign = {
  id: string;
  organizationId: string;
  trafficSourceId: string;
  name: string;
  status: CampaignStatus;
  fallbackUrl: string;
  notes: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateCampaignInput = {
  trafficSourceId: string;
  name: string;
  fallbackUrl: string;
  notes: string;
};

export type UpdateCampaignInput = Partial<CreateCampaignInput> & { status?: CampaignStatus };

export function listCampaigns(): Promise<{ campaigns: Campaign[]; total: number }> {
  return apiFetch("/campaigns");
}

export function getCampaign(id: string): Promise<Campaign> {
  return apiFetch(`/campaigns/${id}`);
}

export function createCampaign(input: CreateCampaignInput): Promise<Campaign> {
  return apiFetch("/campaigns", { method: "POST", body: input });
}

export function updateCampaign(id: string, input: UpdateCampaignInput): Promise<Campaign> {
  return apiFetch(`/campaigns/${id}`, { method: "PATCH", body: input });
}

export function deleteCampaign(id: string): Promise<void> {
  return apiFetch(`/campaigns/${id}`, { method: "DELETE" });
}

export function duplicateCampaign(id: string): Promise<Campaign> {
  return apiFetch(`/campaigns/${id}/duplicate`, { method: "POST" });
}

export function pauseCampaign(id: string): Promise<Campaign> {
  return apiFetch(`/campaigns/${id}/pause`, { method: "POST" });
}

export function activateCampaign(id: string): Promise<Campaign> {
  return apiFetch(`/campaigns/${id}/activate`, { method: "POST" });
}

export function archiveCampaign(id: string): Promise<Campaign> {
  return apiFetch(`/campaigns/${id}`, { method: "PATCH", body: { status: "archived" } });
}

import { apiFetch } from "@/lib/api/client";
import { fetchAllPages } from "@/lib/api/pagination";

export type CampaignStatus = "active" | "paused" | "draft" | "archived";

/** Mirrors apps/internal/campaign.Campaign's JSON shape exactly (id,
 * organizationId, trafficSourceId, name, status, fallbackUrl, notes,
 * externalCampaignId, createdAt, updatedAt) — no trackingDomain/trackingId
 * (that's tracking_links, a separate, not-yet-built entity, see
 * docs/frontend-integration.md) and no clicks/conversions/revenue/spend
 * (those need a bulk analytics rollup that doesn't exist yet either).
 * externalCampaignId is the ad platform's own campaign id (§74/§27-COST,
 * migration 00019) — how the ad-spend sync (POST
 * /traffic-sources/{id}/connection/sync, see ad-account-connections.ts)
 * matches a synced day's spend to this campaign. Empty string for the
 * common case (not funded by a connected ad account, or not mapped yet). */
export type Campaign = {
  id: string;
  organizationId: string;
  trafficSourceId: string;
  name: string;
  status: CampaignStatus;
  fallbackUrl: string;
  notes: string;
  externalCampaignId: string;
  /** The member the campaign belongs to (audit TEAM-1); null when nobody
   * owns it. Without campaign.manage_all only the owner can change it. */
  ownerUserId: string | null;
  createdAt: string;
  updatedAt: string;
};

export type CreateCampaignInput = {
  trafficSourceId: string;
  name: string;
  fallbackUrl: string;
  notes: string;
  externalCampaignId?: string;
};

export type UpdateCampaignInput = Partial<CreateCampaignInput> & { status?: CampaignStatus };

export function listCampaigns(): Promise<{ campaigns: Campaign[]; total: number }> {
  return fetchAllPages<"campaigns", Campaign>("/campaigns", "campaigns");
}

/** GET /campaigns/{id}/diagnostics: clicks of the last `windowDays` UTC
 * days over the tracker's per-IP limit — sent to the campaign's fallback
 * and in no report (audit REL-2). `lastDay` is the last UTC day with any. */
export type CampaignDiagnostics = {
  ipLimited: { windowDays: number; clicks: number; lastDay: string | null };
};

export function getCampaignDiagnostics(id: string): Promise<CampaignDiagnostics> {
  return apiFetch(`/campaigns/${id}/diagnostics`);
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

/** Assigns the campaign to a member, or to nobody with "" (needs
 * campaign.manage_all). */
export function setCampaignOwner(id: string, userId: string): Promise<Campaign> {
  return apiFetch(`/campaigns/${id}/owner`, { method: "PUT", body: { userId } });
}

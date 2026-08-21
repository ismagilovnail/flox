import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/trafficsource's real shape (Phase: Traffic
 * Sources CRUD) — grew from Phase 27's read-only id/name/type/status
 * picker slice into the full entity. `type` stays a free string
 * server-side (traffic_sources.type has no CHECK constraint, per its own
 * migration comment) — SOURCE_TYPES below is a client-side UX vocabulary,
 * not a backend enum. */
export type SourceType = "Facebook" | "TikTok" | "Google" | "Native Ads" | "Push" | "SEO" | "Influencer" | "Email" | "Other";

export const SOURCE_TYPES: SourceType[] = [
  "Facebook",
  "TikTok",
  "Google",
  "Native Ads",
  "Push",
  "SEO",
  "Influencer",
  "Email",
  "Other",
];

/** Matches traffic_sources.cost_integration's CHECK constraint exactly.
 * Records intent only — actual per-day amounts live in
 * apps/internal/cost's cost_entries, entered through a campaign's Cost
 * tab regardless of what a source's integration is set to. */
export type CostIntegration = "none" | "manual" | "facebook_ads" | "tiktok_ads";

export const COST_INTEGRATION_LABELS: Record<CostIntegration, string> = {
  none: "Not connected",
  manual: "Manual entry",
  facebook_ads: "Facebook Ads (OAuth)",
  tiktok_ads: "TikTok Ads (OAuth)",
};

export type SourceStatus = "active" | "paused" | "archived";

export type TrafficSource = {
  id: string;
  organizationId: string;
  name: string;
  type: string;
  trackingTemplate: string;
  costIntegration: CostIntegration;
  status: SourceStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreateTrafficSourceInput = {
  name: string;
  type: string;
  trackingTemplate: string;
  costIntegration: CostIntegration;
};

export type UpdateTrafficSourceInput = Partial<CreateTrafficSourceInput> & { status?: SourceStatus };

export function listTrafficSources(): Promise<{ trafficSources: TrafficSource[] }> {
  return apiFetch("/traffic-sources");
}

export function getTrafficSource(id: string): Promise<TrafficSource> {
  return apiFetch(`/traffic-sources/${id}`);
}

export function createTrafficSource(input: CreateTrafficSourceInput): Promise<TrafficSource> {
  return apiFetch("/traffic-sources", { method: "POST", body: input });
}

export function updateTrafficSource(id: string, input: UpdateTrafficSourceInput): Promise<TrafficSource> {
  return apiFetch(`/traffic-sources/${id}`, { method: "PATCH", body: input });
}

export function deleteTrafficSource(id: string): Promise<void> {
  return apiFetch(`/traffic-sources/${id}`, { method: "DELETE" });
}

export function duplicateTrafficSource(id: string): Promise<TrafficSource> {
  return apiFetch(`/traffic-sources/${id}/duplicate`, { method: "POST" });
}

export function pauseTrafficSource(id: string): Promise<TrafficSource> {
  return apiFetch(`/traffic-sources/${id}/pause`, { method: "POST" });
}

export function activateTrafficSource(id: string): Promise<TrafficSource> {
  return apiFetch(`/traffic-sources/${id}/activate`, { method: "POST" });
}

export function archiveTrafficSource(id: string): Promise<TrafficSource> {
  return apiFetch(`/traffic-sources/${id}`, { method: "PATCH", body: { status: "archived" } });
}

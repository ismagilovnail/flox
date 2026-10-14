import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/trackinglink's real shape. */
export type TrackingLinkStatus = "active" | "paused" | "archived";

export type TrackingLink = {
  id: string;
  organizationId: string;
  campaignId: string;
  domainId: string;
  domain: string;
  slug: string;
  /** The domain serves a valid certificate, so url is https. */
  secure: boolean;
  url: string;
  status: TrackingLinkStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreateTrackingLinkInput = {
  domainId: string;
  slug: string;
};

export function listTrackingLinks(campaignId: string): Promise<{ trackingLinks: TrackingLink[] }> {
  return apiFetch(`/campaigns/${campaignId}/tracking-links`);
}

/** The organization's tracking links across campaigns, optionally of one
 * status — the campaigns list's "Copy tracking URL" (one request). */
export function listOrgTrackingLinks(status?: TrackingLinkStatus): Promise<{ trackingLinks: TrackingLink[] }> {
  return apiFetch("/tracking-links", { searchParams: status ? { status } : undefined });
}

export function createTrackingLink(campaignId: string, input: CreateTrackingLinkInput): Promise<TrackingLink> {
  return apiFetch(`/campaigns/${campaignId}/tracking-links`, { method: "POST", body: input });
}

export function updateTrackingLinkStatus(
  campaignId: string,
  id: string,
  status: TrackingLinkStatus,
): Promise<TrackingLink> {
  return apiFetch(`/campaigns/${campaignId}/tracking-links/${id}`, { method: "PATCH", body: { status } });
}

export function deleteTrackingLink(campaignId: string, id: string): Promise<void> {
  return apiFetch(`/campaigns/${campaignId}/tracking-links/${id}`, { method: "DELETE" });
}

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createTrackingLink,
  deleteTrackingLink,
  listOrgTrackingLinks,
  listTrackingLinks,
  type TrackingLink,
  updateTrackingLinkStatus,
  type CreateTrackingLinkInput,
  type TrackingLinkStatus,
} from "@/lib/api/tracking-links";

const trackingLinksKey = (campaignId: string) => ["tracking-links", campaignId] as const;
// Every mutation below invalidates this prefix: the campaign's own list and
// the organization-wide one (useActiveTrackingLinksByCampaign).
const allTrackingLinksKey = ["tracking-links"] as const;

export function useTrackingLinks(campaignId: string) {
  return useQuery({
    queryKey: trackingLinksKey(campaignId),
    queryFn: () => listTrackingLinks(campaignId),
    enabled: !!campaignId,
  });
}

/** The organization's active tracking links grouped by campaign id — one
 * shared query however many campaign rows ask. */
export function useActiveTrackingLinksByCampaign() {
  return useQuery({
    queryKey: [...allTrackingLinksKey, "@org", "active"],
    queryFn: () => listOrgTrackingLinks("active"),
    select: (data) => {
      const byCampaign = new Map<string, TrackingLink[]>();
      for (const l of data.trackingLinks) byCampaign.set(l.campaignId, [...(byCampaign.get(l.campaignId) ?? []), l]);
      return byCampaign;
    },
  });
}

export function useCreateTrackingLink(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTrackingLinkInput) => createTrackingLink(campaignId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: allTrackingLinksKey }),
  });
}

export function useUpdateTrackingLinkStatus(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: TrackingLinkStatus }) =>
      updateTrackingLinkStatus(campaignId, id, status),
    onSuccess: () => qc.invalidateQueries({ queryKey: allTrackingLinksKey }),
  });
}

export function useDeleteTrackingLink(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteTrackingLink(campaignId, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: allTrackingLinksKey }),
  });
}

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createTrackingLink,
  deleteTrackingLink,
  listTrackingLinks,
  updateTrackingLinkStatus,
  type CreateTrackingLinkInput,
  type TrackingLinkStatus,
} from "@/lib/api/tracking-links";

const trackingLinksKey = (campaignId: string) => ["tracking-links", campaignId] as const;

export function useTrackingLinks(campaignId: string) {
  return useQuery({
    queryKey: trackingLinksKey(campaignId),
    queryFn: () => listTrackingLinks(campaignId),
    enabled: !!campaignId,
  });
}

export function useCreateTrackingLink(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTrackingLinkInput) => createTrackingLink(campaignId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: trackingLinksKey(campaignId) }),
  });
}

export function useUpdateTrackingLinkStatus(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: TrackingLinkStatus }) =>
      updateTrackingLinkStatus(campaignId, id, status),
    onSuccess: () => qc.invalidateQueries({ queryKey: trackingLinksKey(campaignId) }),
  });
}

export function useDeleteTrackingLink(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteTrackingLink(campaignId, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: trackingLinksKey(campaignId) }),
  });
}

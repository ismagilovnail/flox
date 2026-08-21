"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createStreamSet,
  deleteStreamSet,
  duplicateStreamSet,
  listStreamSets,
  reorderStreamSets,
  updateStreamSet,
  type CreateStreamSetInput,
  type UpdateStreamSetInput,
} from "@/lib/api/stream-sets";

const streamSetsKey = (campaignId: string) => ["stream-sets", campaignId] as const;

export function useStreamSets(campaignId: string) {
  return useQuery({
    queryKey: streamSetsKey(campaignId),
    queryFn: () => listStreamSets(campaignId),
    enabled: !!campaignId,
  });
}

export function useCreateStreamSet(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateStreamSetInput) => createStreamSet(campaignId, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: streamSetsKey(campaignId) }),
  });
}

export function useUpdateStreamSet(campaignId: string, id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateStreamSetInput) => updateStreamSet(campaignId, id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: streamSetsKey(campaignId) }),
  });
}

export function useDeleteStreamSet(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteStreamSet(campaignId, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: streamSetsKey(campaignId) }),
  });
}

export function useDuplicateStreamSet(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicateStreamSet(campaignId, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: streamSetsKey(campaignId) }),
  });
}

export function useReorderStreamSets(campaignId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (orderedIds: string[]) => reorderStreamSets(campaignId, orderedIds),
    onSuccess: () => qc.invalidateQueries({ queryKey: streamSetsKey(campaignId) }),
  });
}

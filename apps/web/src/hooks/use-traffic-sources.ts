"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { usePermission } from "@/hooks/use-auth";
import {
  activateTrafficSource,
  archiveTrafficSource,
  createTrafficSource,
  duplicateTrafficSource,
  getTrafficSource,
  listTrafficSources,
  pauseTrafficSource,
  updateTrafficSource,
  type CreateTrafficSourceInput,
  type UpdateTrafficSourceInput,
} from "@/lib/api/traffic-sources";

const sourcesKey = ["traffic-sources"] as const;
const sourceKey = (id: string) => ["traffic-sources", id] as const;

/** Empty without source.read (a Viewer) rather than a 403 — see useOffers. */
export function useTrafficSources() {
  const canRead = usePermission("source.read");
  return useQuery({
    queryKey: sourcesKey,
    queryFn: canRead ? listTrafficSources : async () => ({ trafficSources: [], total: 0 }),
  });
}

export function useTrafficSource(id: string) {
  const canRead = usePermission("source.read");
  return useQuery({ queryKey: sourceKey(id), queryFn: () => getTrafficSource(id), enabled: !!id && canRead });
}

/** Every mutation invalidates the list — same "refetch rather than
 * hand-patch the cache" choice hooks/use-campaigns.ts made, for the same
 * reason: source counts per org are small, so it's simpler and just as
 * fast, and it can never drift from what the server actually did. */
export function useCreateTrafficSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTrafficSourceInput) => createTrafficSource(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: sourcesKey }),
  });
}

export function useUpdateTrafficSource(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateTrafficSourceInput) => updateTrafficSource(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: sourcesKey });
      qc.invalidateQueries({ queryKey: sourceKey(id) });
    },
  });
}

export function useDuplicateTrafficSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicateTrafficSource(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: sourcesKey }),
  });
}

export function usePauseTrafficSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pauseTrafficSource(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: sourcesKey });
      qc.invalidateQueries({ queryKey: sourceKey(id) });
    },
  });
}

export function useActivateTrafficSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activateTrafficSource(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: sourcesKey });
      qc.invalidateQueries({ queryKey: sourceKey(id) });
    },
  });
}

export function useArchiveTrafficSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveTrafficSource(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: sourcesKey });
      qc.invalidateQueries({ queryKey: sourceKey(id) });
    },
  });
}

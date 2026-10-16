"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { usePermission } from "@/hooks/use-auth";
import {
  createEventMapping,
  deleteEventMapping,
  listEventMappings,
  listWaitingStatuses,
  type CreateEventMappingInput,
} from "@/lib/api/event-mappings";

const eventMappingsKey = ["event-mappings"] as const;
const waitingStatusesKey = ["event-mappings", "waiting"] as const;

export function useEventMappings() {
  const canRead = usePermission("offer.read");
  return useQuery({
    queryKey: eventMappingsKey,
    queryFn: canRead ? listEventMappings : async () => ({ eventMappings: [], total: 0 }),
  });
}

/** Statuses postbacks are waiting to have mapped. Refetched every minute:
 * a mapping saved now clears its row once apps/worker has recorded the
 * postbacks (within 30 s). */
export function useWaitingStatuses() {
  const canRead = usePermission("offer.read");
  return useQuery({
    queryKey: waitingStatusesKey,
    queryFn: canRead ? listWaitingStatuses : async () => ({ statuses: [] }),
    refetchInterval: 60_000,
  });
}

export function useCreateEventMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateEventMappingInput) => createEventMapping(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: eventMappingsKey }),
  });
}

export function useDeleteEventMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteEventMapping(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: eventMappingsKey }),
  });
}

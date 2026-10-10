"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { usePermission } from "@/hooks/use-auth";
import {
  createEventMapping,
  deleteEventMapping,
  listEventMappings,
  type CreateEventMappingInput,
} from "@/lib/api/event-mappings";

const eventMappingsKey = ["event-mappings"] as const;

export function useEventMappings() {
  const canRead = usePermission("offer.read");
  return useQuery({
    queryKey: eventMappingsKey,
    queryFn: canRead ? listEventMappings : async () => ({ eventMappings: [], total: 0 }),
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

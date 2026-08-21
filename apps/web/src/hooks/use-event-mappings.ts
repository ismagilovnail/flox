"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createEventMapping,
  deleteEventMapping,
  listEventMappings,
  type CreateEventMappingInput,
} from "@/lib/api/event-mappings";

const eventMappingsKey = ["event-mappings"] as const;

export function useEventMappings() {
  return useQuery({ queryKey: eventMappingsKey, queryFn: listEventMappings });
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

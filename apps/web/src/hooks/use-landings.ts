"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activateLanding,
  archiveLanding,
  createLanding,
  duplicateLanding,
  getLanding,
  listLandings,
  pauseLanding,
  updateLanding,
  type CreateLandingInput,
  type UpdateLandingInput,
} from "@/lib/api/landings";

const landingsKey = ["landings"] as const;
const landingKey = (id: string) => ["landings", id] as const;

export function useLandings() {
  return useQuery({ queryKey: landingsKey, queryFn: listLandings });
}

export function useLanding(id: string) {
  return useQuery({ queryKey: landingKey(id), queryFn: () => getLanding(id), enabled: !!id });
}

export function useCreateLanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateLandingInput) => createLanding(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: landingsKey }),
  });
}

export function useUpdateLanding(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateLandingInput) => updateLanding(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: landingsKey });
      qc.invalidateQueries({ queryKey: landingKey(id) });
    },
  });
}

export function useDuplicateLanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicateLanding(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: landingsKey }),
  });
}

export function usePauseLanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pauseLanding(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: landingsKey });
      qc.invalidateQueries({ queryKey: landingKey(id) });
    },
  });
}

export function useActivateLanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activateLanding(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: landingsKey });
      qc.invalidateQueries({ queryKey: landingKey(id) });
    },
  });
}

export function useArchiveLanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveLanding(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: landingsKey });
      qc.invalidateQueries({ queryKey: landingKey(id) });
    },
  });
}

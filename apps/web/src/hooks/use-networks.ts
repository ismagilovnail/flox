"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activateNetwork,
  archiveNetwork,
  createNetwork,
  duplicateNetwork,
  getNetwork,
  listNetworks,
  pauseNetwork,
  regeneratePostbackSecret,
  updateNetwork,
  type CreateNetworkInput,
  type UpdateNetworkInput,
} from "@/lib/api/networks";

const networksKey = ["networks"] as const;
const networkKey = (id: string) => ["networks", id] as const;

export function useNetworks() {
  return useQuery({ queryKey: networksKey, queryFn: listNetworks });
}

export function useNetwork(id: string) {
  return useQuery({ queryKey: networkKey(id), queryFn: () => getNetwork(id), enabled: !!id });
}

export function useCreateNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateNetworkInput) => createNetwork(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: networksKey }),
  });
}

export function useUpdateNetwork(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateNetworkInput) => updateNetwork(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: networksKey });
      qc.invalidateQueries({ queryKey: networkKey(id) });
    },
  });
}

export function useDuplicateNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicateNetwork(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: networksKey }),
  });
}

export function usePauseNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pauseNetwork(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: networksKey });
      qc.invalidateQueries({ queryKey: networkKey(id) });
    },
  });
}

export function useActivateNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activateNetwork(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: networksKey });
      qc.invalidateQueries({ queryKey: networkKey(id) });
    },
  });
}

export function useRegeneratePostbackSecret() {
  return useMutation({
    mutationFn: (id: string) => regeneratePostbackSecret(id),
    // Nothing cached needs invalidating — Network's own fields never
    // change, and the secret itself is never part of any cached query
    // (it only ever appears in this mutation's own response).
  });
}

export function useArchiveNetwork() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveNetwork(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: networksKey });
      qc.invalidateQueries({ queryKey: networkKey(id) });
    },
  });
}

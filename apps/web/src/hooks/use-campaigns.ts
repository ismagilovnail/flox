"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activateCampaign,
  archiveCampaign,
  createCampaign,
  duplicateCampaign,
  getCampaign,
  listCampaigns,
  pauseCampaign,
  updateCampaign,
  type CreateCampaignInput,
  type UpdateCampaignInput,
} from "@/lib/api/campaigns";

const campaignsKey = ["campaigns"] as const;
const campaignKey = (id: string) => ["campaigns", id] as const;

export function useCampaigns() {
  return useQuery({ queryKey: campaignsKey, queryFn: listCampaigns });
}

export function useCampaign(id: string) {
  return useQuery({ queryKey: campaignKey(id), queryFn: () => getCampaign(id), enabled: !!id });
}

/** Every mutation invalidates both the list and the one detail query it
 * touched — campaigns' own data volume is small enough that "refetch the
 * list" is simpler and just as fast as hand-patching the cache, and it
 * can never drift from what the server actually did. */
export function useCreateCampaign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateCampaignInput) => createCampaign(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: campaignsKey }),
  });
}

export function useUpdateCampaign(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateCampaignInput) => updateCampaign(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: campaignsKey });
      qc.invalidateQueries({ queryKey: campaignKey(id) });
    },
  });
}

export function useDuplicateCampaign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicateCampaign(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: campaignsKey }),
  });
}

export function usePauseCampaign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pauseCampaign(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: campaignsKey });
      qc.invalidateQueries({ queryKey: campaignKey(id) });
    },
  });
}

export function useActivateCampaign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activateCampaign(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: campaignsKey });
      qc.invalidateQueries({ queryKey: campaignKey(id) });
    },
  });
}

export function useArchiveCampaign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveCampaign(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: campaignsKey });
      qc.invalidateQueries({ queryKey: campaignKey(id) });
    },
  });
}

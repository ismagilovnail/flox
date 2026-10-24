"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { usePermission } from "@/hooks/use-auth";
import {
  activateNetwork,
  archiveNetwork,
  createNetwork,
  deleteShopify,
  duplicateNetwork,
  getNetwork,
  getNetworkDiagnostics,
  getShopify,
  updateShopify,
  type ShopifyInput,
  listNetworks,
  pauseNetwork,
  regeneratePostbackSecret,
  updateNetwork,
  type CreateNetworkInput,
  type UpdateNetworkInput,
} from "@/lib/api/networks";

const networksKey = ["networks"] as const;
const networkKey = (id: string) => ["networks", id] as const;

/** Empty without offer.read — see useOffers. */
export function useNetworks() {
  const canRead = usePermission("offer.read");
  return useQuery({ queryKey: networksKey, queryFn: canRead ? listNetworks : async () => ({ networks: [], total: 0 }) });
}

export function useNetwork(id: string) {
  const canRead = usePermission("offer.read");
  return useQuery({ queryKey: networkKey(id), queryFn: () => getNetwork(id), enabled: !!id && canRead });
}

export function useNetworkDiagnostics(id: string) {
  const canRead = usePermission("offer.read");
  return useQuery({
    queryKey: [...networkKey(id), "diagnostics"],
    queryFn: () => getNetworkDiagnostics(id),
    enabled: !!id && canRead,
  });
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

const shopifyKey = (networkId: string) => ["networks", networkId, "shopify"] as const;

/** An own-site source's Shopify store (audit UNI-4 part 3). */
export function useShopify(networkId: string) {
  return useQuery({ queryKey: shopifyKey(networkId), queryFn: () => getShopify(networkId), enabled: !!networkId });
}

export function useUpdateShopify(networkId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ShopifyInput) => updateShopify(networkId, input),
    onSuccess: (data) => qc.setQueryData(shopifyKey(networkId), data),
  });
}

export function useDeleteShopify(networkId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => deleteShopify(networkId),
    onSuccess: () => qc.invalidateQueries({ queryKey: shopifyKey(networkId) }),
  });
}

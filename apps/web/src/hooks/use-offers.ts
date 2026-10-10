"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { usePermission } from "@/hooks/use-auth";
import {
  activateOffer,
  archiveOffer,
  createOffer,
  duplicateOffer,
  getOffer,
  listOfferCaps,
  listOffers,
  pauseOffer,
  updateOffer,
  type CreateOfferInput,
  type UpdateOfferInput,
} from "@/lib/api/offers";

const offersKey = ["offers"] as const;
const offerKey = (id: string) => ["offers", id] as const;

/** The cap snapshot refreshes every 30 s on the server. */
export function useOfferCaps() {
  const canRead = usePermission("offer.read");
  return useQuery({
    queryKey: ["offer-caps"],
    queryFn: canRead ? listOfferCaps : async () => ({ caps: [], available: false }),
    refetchInterval: canRead ? 60_000 : false,
  });
}

/** Without offer.read (a Viewer) the list is empty rather than a 403:
 * pages that only label things with offer names (campaigns, analytics)
 * keep working and show ids instead. */
export function useOffers() {
  const canRead = usePermission("offer.read");
  return useQuery({ queryKey: offersKey, queryFn: canRead ? listOffers : async () => ({ offers: [], total: 0 }) });
}

export function useOffer(id: string) {
  const canRead = usePermission("offer.read");
  return useQuery({ queryKey: offerKey(id), queryFn: () => getOffer(id), enabled: !!id && canRead });
}

export function useCreateOffer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateOfferInput) => createOffer(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: offersKey }),
  });
}

export function useUpdateOffer(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateOfferInput) => updateOffer(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: offersKey });
      qc.invalidateQueries({ queryKey: offerKey(id) });
    },
  });
}

export function useDuplicateOffer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicateOffer(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: offersKey }),
  });
}

export function usePauseOffer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pauseOffer(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: offersKey });
      qc.invalidateQueries({ queryKey: offerKey(id) });
    },
  });
}

export function useActivateOffer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activateOffer(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: offersKey });
      qc.invalidateQueries({ queryKey: offerKey(id) });
    },
  });
}

export function useArchiveOffer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archiveOffer(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: offersKey });
      qc.invalidateQueries({ queryKey: offerKey(id) });
    },
  });
}

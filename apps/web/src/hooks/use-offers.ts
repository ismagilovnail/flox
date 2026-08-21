"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activateOffer,
  archiveOffer,
  createOffer,
  duplicateOffer,
  getOffer,
  listOffers,
  pauseOffer,
  updateOffer,
  type CreateOfferInput,
  type UpdateOfferInput,
} from "@/lib/api/offers";

const offersKey = ["offers"] as const;
const offerKey = (id: string) => ["offers", id] as const;

export function useOffers() {
  return useQuery({ queryKey: offersKey, queryFn: listOffers });
}

export function useOffer(id: string) {
  return useQuery({ queryKey: offerKey(id), queryFn: () => getOffer(id), enabled: !!id });
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

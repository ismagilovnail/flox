"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activatePixel,
  archivePixel,
  createPixel,
  duplicatePixel,
  getPixel,
  getPixelEventNameDefaults,
  listPixelDeliveries,
  listPixels,
  pausePixel,
  sendPixelTestEvent,
  updatePixel,
  type CreatePixelInput,
  type UpdatePixelInput,
} from "@/lib/api/pixels";

const pixelsKey = ["pixels"] as const;
const pixelKey = (id: string) => ["pixels", id] as const;

export function usePixels() {
  return useQuery({ queryKey: pixelsKey, queryFn: listPixels });
}

export function usePixel(id: string) {
  return useQuery({ queryKey: pixelKey(id), queryFn: () => getPixel(id), enabled: !!id });
}

/** The pixel's recent deliveries, refreshed while the page is open so a
 * test conversion shows up without a reload. */
export function usePixelDeliveries(id: string) {
  return useQuery({
    queryKey: ["pixels", id, "deliveries"],
    queryFn: () => listPixelDeliveries(id),
    enabled: !!id,
    refetchInterval: 15_000,
  });
}

export function usePixelEventNameDefaults() {
  return useQuery({ queryKey: ["pixels", "event-names"], queryFn: getPixelEventNameDefaults, staleTime: Infinity });
}

export function useCreatePixel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreatePixelInput) => createPixel(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: pixelsKey }),
  });
}

export function useUpdatePixel(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdatePixelInput) => updatePixel(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: pixelsKey });
      qc.invalidateQueries({ queryKey: pixelKey(id) });
    },
  });
}

export function useDuplicatePixel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicatePixel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: pixelsKey }),
  });
}

export function usePausePixel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pausePixel(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: pixelsKey });
      qc.invalidateQueries({ queryKey: pixelKey(id) });
    },
  });
}

export function useActivatePixel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activatePixel(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: pixelsKey });
      qc.invalidateQueries({ queryKey: pixelKey(id) });
    },
  });
}

export function useArchivePixel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archivePixel(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: pixelsKey });
      qc.invalidateQueries({ queryKey: pixelKey(id) });
    },
  });
}

/** Sends a test event; the delivery log then shows it. */
export function useSendPixelTestEvent(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { eventType: string; confirmReal: boolean }) => sendPixelTestEvent(id, input),
    onSettled: () => qc.invalidateQueries({ queryKey: ["pixels", id, "deliveries"] }),
  });
}

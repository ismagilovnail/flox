"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activatePixel,
  archivePixel,
  createPixel,
  duplicatePixel,
  getPixel,
  listPixels,
  pausePixel,
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

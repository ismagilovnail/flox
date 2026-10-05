"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activatePwa,
  archivePwa,
  createPwa,
  duplicatePwa,
  getPwa,
  getPwaComments,
  listPwas,
  pausePwa,
  replacePwaComments,
  updatePwa,
  type CreatePwaInput,
  type PwaComment,
  type UpdatePwaInput,
} from "@/lib/api/pwa";

const pwasKey = ["pwas"] as const;
const pwaKey = (id: string) => ["pwas", id] as const;

export function usePwas() {
  return useQuery({ queryKey: pwasKey, queryFn: listPwas });
}

export function usePwa(id: string) {
  return useQuery({ queryKey: pwaKey(id), queryFn: () => getPwa(id), enabled: !!id });
}

export function useCreatePwa() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreatePwaInput) => createPwa(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: pwasKey }),
  });
}

export function useUpdatePwa(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdatePwaInput) => updatePwa(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: pwasKey });
      qc.invalidateQueries({ queryKey: pwaKey(id) });
    },
  });
}

export function useDuplicatePwa() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicatePwa(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: pwasKey }),
  });
}

export function usePausePwa() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pausePwa(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: pwasKey });
      qc.invalidateQueries({ queryKey: pwaKey(id) });
    },
  });
}

export function useActivatePwa() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activatePwa(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: pwasKey });
      qc.invalidateQueries({ queryKey: pwaKey(id) });
    },
  });
}

export function useArchivePwa() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archivePwa(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: pwasKey });
      qc.invalidateQueries({ queryKey: pwaKey(id) });
    },
  });
}

const pwaCommentsKey = (id: string) => ["pwas", id, "comments"] as const;

export function usePwaComments(id: string) {
  return useQuery({ queryKey: pwaCommentsKey(id), queryFn: () => getPwaComments(id), enabled: !!id });
}

export function useReplacePwaComments() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, comments }: { id: string; comments: PwaComment[] }) => replacePwaComments(id, comments),
    onSuccess: (_data, { id }) => qc.invalidateQueries({ queryKey: pwaCommentsKey(id) }),
  });
}

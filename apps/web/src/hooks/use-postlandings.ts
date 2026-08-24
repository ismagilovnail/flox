"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  activatePostlanding,
  archivePostlanding,
  createPostlanding,
  duplicatePostlanding,
  getPostlanding,
  listPostlandings,
  pausePostlanding,
  updatePostlanding,
  type CreatePostlandingInput,
  type UpdatePostlandingInput,
} from "@/lib/api/postlanding";

const postlandingsKey = ["postlandings"] as const;
const postlandingKey = (id: string) => ["postlandings", id] as const;

export function usePostlandings() {
  return useQuery({ queryKey: postlandingsKey, queryFn: listPostlandings });
}

export function usePostlanding(id: string) {
  return useQuery({ queryKey: postlandingKey(id), queryFn: () => getPostlanding(id), enabled: !!id });
}

export function useCreatePostlanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreatePostlandingInput) => createPostlanding(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: postlandingsKey }),
  });
}

export function useUpdatePostlanding(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdatePostlandingInput) => updatePostlanding(id, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: postlandingsKey });
      qc.invalidateQueries({ queryKey: postlandingKey(id) });
    },
  });
}

export function useDuplicatePostlanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => duplicatePostlanding(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: postlandingsKey }),
  });
}

export function usePausePostlanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pausePostlanding(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: postlandingsKey });
      qc.invalidateQueries({ queryKey: postlandingKey(id) });
    },
  });
}

export function useActivatePostlanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => activatePostlanding(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: postlandingsKey });
      qc.invalidateQueries({ queryKey: postlandingKey(id) });
    },
  });
}

export function useArchivePostlanding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => archivePostlanding(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: postlandingsKey });
      qc.invalidateQueries({ queryKey: postlandingKey(id) });
    },
  });
}

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { createApiKey, listApiKeys, revokeApiKey, rotateApiKey } from "@/lib/api/api-keys";
import type { ApiKeyScope } from "@/lib/api-keys";

const apiKeysKey = ["api-keys"] as const;

export function useApiKeys() {
  return useQuery({ queryKey: apiKeysKey, queryFn: listApiKeys });
}

export function useCreateApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; scope: ApiKeyScope }) => createApiKey(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: apiKeysKey }),
  });
}

export function useRotateApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => rotateApiKey(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: apiKeysKey }),
  });
}

export function useRevokeApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => revokeApiKey(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: apiKeysKey }),
  });
}

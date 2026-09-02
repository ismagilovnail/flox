"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createDomain,
  deleteDomain,
  listDomains,
  updateDomain,
  type CreateDomainInput,
  type UpdateDomainInput,
} from "@/lib/api/domains";

const domainsKey = ["domains"] as const;

export function useDomains() {
  return useQuery({ queryKey: domainsKey, queryFn: listDomains });
}

export function useCreateDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDomainInput) => createDomain(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: domainsKey }),
  });
}

export function useUpdateDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateDomainInput }) => updateDomain(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: domainsKey }),
  });
}

export function useDeleteDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteDomain(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: domainsKey }),
  });
}

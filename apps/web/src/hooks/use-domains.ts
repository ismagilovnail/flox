"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createDomain,
  deleteDomain,
  getDomain,
  listDomains,
  updateDomain,
  type CreateDomainInput,
  type UpdateDomainInput,
} from "@/lib/api/domains";

const domainsKey = ["domains"] as const;
const domainKey = (id: string) => ["domains", id] as const;

export function useDomains() {
  return useQuery({ queryKey: domainsKey, queryFn: listDomains });
}

export function useDomain(id: string) {
  return useQuery({ queryKey: domainKey(id), queryFn: () => getDomain(id), enabled: !!id });
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
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: domainsKey });
      qc.invalidateQueries({ queryKey: domainKey(id) });
    },
  });
}

export function useDeleteDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteDomain(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: domainsKey }),
  });
}

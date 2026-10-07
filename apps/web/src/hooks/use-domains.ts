"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createDomain,
  deleteDomain,
  checkDomainSSL,
  getDomain,
  listDomains,
  renewDomain,
  syncDomainDNS,
  updateDomain,
  verifyDomain,
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

function invalidateAfter(qc: ReturnType<typeof useQueryClient>, id: string) {
  qc.invalidateQueries({ queryKey: domainsKey });
  qc.invalidateQueries({ queryKey: domainKey(id) });
}

export function useVerifyDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => verifyDomain(id),
    onSuccess: (_data, id) => invalidateAfter(qc, id),
  });
}

export function useRenewDomain() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, years }: { id: string; years: number }) => renewDomain(id, years),
    onSuccess: (_data, { id }) => invalidateAfter(qc, id),
  });
}

export function useSyncDomainDNS() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => syncDomainDNS(id),
    onSuccess: (_data, id) => invalidateAfter(qc, id),
  });
}

export function useCheckDomainSSL() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => checkDomainSSL(id),
    onSuccess: (_data, id) => invalidateAfter(qc, id),
  });
}

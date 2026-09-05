"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { connectIntegration, disconnectIntegration, getIntegrations, verifyIntegration } from "@/lib/api/integrations";
import type { CredentialProvider } from "@/lib/integrations";

const integrationsKey = ["integrations"] as const;

export function useIntegrations() {
  return useQuery({ queryKey: integrationsKey, queryFn: getIntegrations });
}

export function useConnectIntegration() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ provider, fields }: { provider: CredentialProvider; fields: Record<string, string> }) =>
      connectIntegration(provider, fields),
    onSuccess: () => qc.invalidateQueries({ queryKey: integrationsKey }),
  });
}

export function useVerifyIntegration() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (provider: CredentialProvider) => verifyIntegration(provider),
    onSuccess: () => qc.invalidateQueries({ queryKey: integrationsKey }),
  });
}

export function useDisconnectIntegration() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (provider: CredentialProvider) => disconnectIntegration(provider),
    onSuccess: () => qc.invalidateQueries({ queryKey: integrationsKey }),
  });
}

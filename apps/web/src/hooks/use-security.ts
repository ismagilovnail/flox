"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getSecuritySettings, updateSecuritySettings } from "@/lib/api/security";
import type { SecuritySettings } from "@/lib/security";

const securityKey = ["security"] as const;

export function useSecuritySettings() {
  return useQuery({ queryKey: securityKey, queryFn: getSecuritySettings });
}

export function useUpdateSecuritySettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<SecuritySettings>) => updateSecuritySettings(input),
    onSuccess: (updated) => qc.setQueryData(securityKey, updated),
  });
}

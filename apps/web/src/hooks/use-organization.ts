"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getOrganization, updateOrganization } from "@/lib/api/organization";
import type { OrganizationSettings } from "@/lib/organization";

const organizationKey = ["organization"] as const;

export function useOrganization() {
  return useQuery({ queryKey: organizationKey, queryFn: getOrganization });
}

export function useUpdateOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<Pick<OrganizationSettings, "name" | "timezone" | "currency">>) => updateOrganization(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: organizationKey }),
  });
}

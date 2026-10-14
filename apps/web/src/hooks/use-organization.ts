"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getOrganization, updateOrganization } from "@/lib/api/organization";
import type { OrganizationSettings } from "@/lib/organization";

const organizationKey = ["organization"] as const;

export function useOrganization() {
  return useQuery({ queryKey: organizationKey, queryFn: getOrganization });
}

/** The currency report money comes out in (§50-FX) — the API converts by
 * this same organization setting. USD until the organization loads. */
export function useReportingCurrency(): string {
  return useOrganization().data?.currency || "USD";
}

export function useUpdateOrganization() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: Partial<Pick<OrganizationSettings, "name" | "timezone" | "currency">>) => updateOrganization(input),
    // A new currency changes every report's money: refetch them all.
    onSuccess: (_data, input) => qc.invalidateQueries(input.currency ? undefined : { queryKey: organizationKey }),
  });
}

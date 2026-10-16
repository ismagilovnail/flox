"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  cancelOrganizationDeletion,
  getOrganization,
  getOrganizationDeletion,
  scheduleOrganizationDeletion,
  updateOrganization,
} from "@/lib/api/organization";
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

const deletionKey = ["organization", "deletion"] as const;

/** The organization's scheduled deletion — the app-wide banner and the
 * settings card read it. */
export function useOrganizationDeletion() {
  return useQuery({ queryKey: deletionKey, queryFn: getOrganizationDeletion, staleTime: 60_000 });
}

export function useScheduleOrganizationDeletion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: scheduleOrganizationDeletion,
    onSuccess: (data) => qc.setQueryData(deletionKey, data),
  });
}

export function useCancelOrganizationDeletion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: cancelOrganizationDeletion,
    onSuccess: (data) => qc.setQueryData(deletionKey, data),
  });
}

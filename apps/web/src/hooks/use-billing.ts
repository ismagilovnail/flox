"use client";

import { useQuery } from "@tanstack/react-query";

import { getBilling, listPlans } from "@/lib/api/billing";

export const billingKey = ["billing"] as const;

/** The plan and this month's usage — read by the banner on every page,
 * so it is kept for a few minutes rather than refetched per page. `fresh`
 * (the Plan tab) reads it again on open: counts change on other pages. */
export function useBilling({ fresh = false }: { fresh?: boolean } = {}) {
  return useQuery({ queryKey: billingKey, queryFn: getBilling, staleTime: fresh ? 0 : 5 * 60_000 });
}

export function usePlans() {
  return useQuery({ queryKey: ["billing", "plans"], queryFn: listPlans, staleTime: 60 * 60_000 });
}

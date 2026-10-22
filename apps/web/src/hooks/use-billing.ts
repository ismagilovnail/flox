"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { checkout, getBilling, getPayment, listPayments, listPlans } from "@/lib/api/billing";

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

/** Payment history: owners and admins (settings.write). */
export function usePayments(enabled: boolean) {
  return useQuery({ queryKey: ["billing", "payments"], queryFn: listPayments, enabled });
}

/** The payment the payer came back from: read again every few seconds
 * while YooKassa has not settled it. */
export function usePayment(id: string | null) {
  return useQuery({
    queryKey: ["billing", "payment", id],
    queryFn: () => getPayment(id as string),
    enabled: !!id,
    refetchInterval: (q) => (q.state.data?.status === "pending" ? 3_000 : false),
  });
}

/** Starts a checkout; the caller sends the browser to its payment page. */
export function useCheckout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: checkout,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["billing", "payments"] }),
  });
}

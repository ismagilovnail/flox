"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  addTransaction,
  approvePayout,
  getReferralDashboard,
  markPayoutPaid,
  rejectPayout,
  requestPayout,
  type AddTransactionInput,
} from "@/lib/api/referral";

const dashboardKey = ["referral"] as const;

export function useReferralDashboard() {
  return useQuery({ queryKey: dashboardKey, queryFn: getReferralDashboard });
}

function useInvalidateReferralDashboard() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: dashboardKey });
}

export function useRequestPayout() {
  const invalidate = useInvalidateReferralDashboard();
  return useMutation({
    mutationFn: (amount: number) => requestPayout(amount),
    onSuccess: invalidate,
  });
}

export function useApprovePayout() {
  const invalidate = useInvalidateReferralDashboard();
  return useMutation({
    mutationFn: (id: string) => approvePayout(id),
    onSuccess: invalidate,
  });
}

export function useRejectPayout() {
  const invalidate = useInvalidateReferralDashboard();
  return useMutation({
    mutationFn: ({ id, note }: { id: string; note: string }) => rejectPayout(id, note),
    onSuccess: invalidate,
  });
}

export function useMarkPayoutPaid() {
  const invalidate = useInvalidateReferralDashboard();
  return useMutation({
    mutationFn: (id: string) => markPayoutPaid(id),
    onSuccess: invalidate,
  });
}

export function useAddTransaction() {
  const invalidate = useInvalidateReferralDashboard();
  return useMutation({
    mutationFn: (input: AddTransactionInput) => addTransaction(input),
    onSuccess: invalidate,
  });
}

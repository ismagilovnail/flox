import { apiFetch } from "@/lib/api/client";
import type { PayoutRequest, ReferralDashboard, ReferralTransaction, ReferralTransactionType } from "@/lib/referral";

export function getReferralDashboard(): Promise<ReferralDashboard> {
  return apiFetch("/referral");
}

export function requestPayout(amount: number): Promise<PayoutRequest> {
  return apiFetch("/referral/payouts", { method: "POST", body: { amount } });
}

export function approvePayout(id: string): Promise<PayoutRequest> {
  return apiFetch(`/referral/payouts/${id}/approve`, { method: "POST" });
}

export function rejectPayout(id: string, note: string): Promise<PayoutRequest> {
  return apiFetch(`/referral/payouts/${id}/reject`, { method: "POST", body: { note } });
}

export function markPayoutPaid(id: string): Promise<PayoutRequest> {
  return apiFetch(`/referral/payouts/${id}/mark-paid`, { method: "POST" });
}

export type AddTransactionInput = {
  type: ReferralTransactionType;
  amount: number;
  description: string;
  relatedSignupId?: string;
};

export function addTransaction(input: AddTransactionInput): Promise<ReferralTransaction> {
  return apiFetch("/referral/transactions", { method: "POST", body: input });
}

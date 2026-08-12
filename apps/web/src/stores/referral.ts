import { create } from "zustand";

import { genId } from "@/lib/id";
import {
  PAYOUT_REQUESTS,
  REFERRAL_TRANSACTIONS,
  REFERRED_SIGNUPS,
  computeBalances,
  type PayoutRequest,
  type ReferralTransaction,
  type ReferredSignup,
} from "@/lib/mock/referral";

type ReferralState = {
  signups: ReferredSignup[];
  transactions: ReferralTransaction[];
  payouts: PayoutRequest[];

  /** Returns the new request's id, or null if the amount is invalid / exceeds the available balance. */
  requestPayout: (amount: number, memberId: string) => string | null;
  approvePayout: (id: string, memberId: string) => boolean;
  rejectPayout: (id: string, memberId: string, note: string) => boolean;
  markPayoutPaid: (id: string, memberId: string) => boolean;
  /** Amount may be negative (a correction downward) — always requires a reason, per §54 audit trail. */
  addAdjustment: (amount: number, description: string, memberId: string) => boolean;
};

export const useReferralStore = create<ReferralState>()((set, get) => ({
  signups: [...REFERRED_SIGNUPS],
  transactions: [...REFERRAL_TRANSACTIONS],
  payouts: [...PAYOUT_REQUESTS],

  requestPayout: (amount, memberId) => {
    if (amount <= 0) return null;
    const { availableBalance } = computeBalances(get().transactions, get().payouts);
    if (amount > availableBalance) return null;

    const id = genId();
    const now = new Date().toISOString();
    const request: PayoutRequest = {
      id,
      amount,
      currency: "USD",
      status: "pending",
      requestedAt: now,
      requestedByMemberId: memberId,
      resolvedAt: null,
      resolvedByMemberId: null,
      note: "",
    };
    set((s) => ({ payouts: [request, ...s.payouts] }));
    return id;
  },

  approvePayout: (id, memberId) => {
    const target = get().payouts.find((p) => p.id === id);
    if (!target || target.status !== "pending") return false;
    const now = new Date().toISOString();
    set((s) => ({
      payouts: s.payouts.map((p) =>
        p.id === id ? { ...p, status: "approved", resolvedAt: now, resolvedByMemberId: memberId } : p,
      ),
    }));
    return true;
  },

  rejectPayout: (id, memberId, note) => {
    const target = get().payouts.find((p) => p.id === id);
    if (!target || target.status !== "pending") return false;
    const now = new Date().toISOString();
    set((s) => ({
      payouts: s.payouts.map((p) =>
        p.id === id ? { ...p, status: "rejected", resolvedAt: now, resolvedByMemberId: memberId, note } : p,
      ),
    }));
    return true;
  },

  markPayoutPaid: (id, memberId) => {
    const target = get().payouts.find((p) => p.id === id);
    if (!target || target.status !== "approved") return false;
    const now = new Date().toISOString();
    const transaction: ReferralTransaction = {
      id: genId(),
      type: "payout_paid",
      amount: -target.amount,
      currency: "USD",
      description: "Payout paid — bank transfer",
      relatedPayoutId: id,
      createdAt: now,
      createdByMemberId: memberId,
    };
    set((s) => ({
      payouts: s.payouts.map((p) =>
        p.id === id ? { ...p, status: "paid", resolvedAt: now, resolvedByMemberId: memberId } : p,
      ),
      transactions: [transaction, ...s.transactions],
    }));
    return true;
  },

  addAdjustment: (amount, description, memberId) => {
    if (amount === 0 || !description.trim()) return false;
    const transaction: ReferralTransaction = {
      id: genId(),
      type: "adjustment",
      amount,
      currency: "USD",
      description: description.trim(),
      createdAt: new Date().toISOString(),
      createdByMemberId: memberId,
    };
    set((s) => ({ transactions: [transaction, ...s.transactions] }));
    return true;
  },
}));

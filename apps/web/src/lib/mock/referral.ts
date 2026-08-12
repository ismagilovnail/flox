/**
 * Referral Program (§30.7) — one referral account per workspace (team-scoped,
 * §36-TENANCY), not per individual member: FLOX's referral program refers
 * other advertisers to the platform itself, so the code/link and balance
 * belong to the tenant, the same scoping level as everything else in the
 * app. Amounts are flat USD — this is FLOX's own commission to the tenant,
 * not tenant traffic revenue, so the §50-FX original-currency/event-date
 * normalization that applies to campaign revenue doesn't apply here.
 *
 * The balance is never stored directly — it's always derived from the
 * append-only `ReferralTransaction` ledger below, which doubles as the
 * §54 audit trail: every credit or debit is one more immutable entry, never
 * an edit to a running total.
 */

import { slugify } from "@/lib/utils";

export function referralCode(orgName: string): string {
  return `${slugify(orgName)}-8f3a`;
}

export function referralLink(orgName: string): string {
  return `https://flox.io/r/${referralCode(orgName)}`;
}

export type SignupStatus = "trial" | "active_customer" | "churned";

export type ReferredSignup = {
  id: string;
  name: string;
  email: string;
  signedUpAt: string;
  status: SignupStatus;
};

export const REFERRED_SIGNUPS: ReferredSignup[] = [
  { id: "ref_signup_1", name: "Marta Kowalski", email: "marta@blueridgemedia.io", signedUpAt: "2026-03-02T00:00:00Z", status: "active_customer" },
  { id: "ref_signup_2", name: "Diego Fernandez", email: "diego@apextraffic.co", signedUpAt: "2026-04-18T00:00:00Z", status: "active_customer" },
  { id: "ref_signup_3", name: "Priya Nair", email: "priya@nairmedia.com", signedUpAt: "2026-05-27T00:00:00Z", status: "trial" },
  { id: "ref_signup_4", name: "Tom Bakker", email: "tom@bakkerads.nl", signedUpAt: "2026-06-14T00:00:00Z", status: "churned" },
  { id: "ref_signup_5", name: "Lena Fischer", email: "lena@fischerperformance.de", signedUpAt: "2026-07-30T00:00:00Z", status: "trial" },
];

export type ReferralTransactionType = "accrual" | "adjustment" | "payout_paid";

export type ReferralTransaction = {
  id: string;
  type: ReferralTransactionType;
  /** Positive = credit to the balance (accrual, positive adjustment); negative = debit (payout paid out, negative adjustment). */
  amount: number;
  currency: "USD";
  description: string;
  relatedSignupId?: string;
  relatedPayoutId?: string;
  createdAt: string;
  createdByMemberId: string;
};

export const REFERRAL_TRANSACTIONS: ReferralTransaction[] = [
  {
    id: "ref_txn_1",
    type: "accrual",
    amount: 50,
    currency: "USD",
    description: "Referral bonus — Marta Kowalski converted to paying customer",
    relatedSignupId: "ref_signup_1",
    createdAt: "2026-03-20T00:00:00Z",
    createdByMemberId: "mem_owner",
  },
  {
    id: "ref_txn_2",
    type: "accrual",
    amount: 50,
    currency: "USD",
    description: "Referral bonus — Diego Fernandez converted to paying customer",
    relatedSignupId: "ref_signup_2",
    createdAt: "2026-05-02T00:00:00Z",
    createdByMemberId: "mem_owner",
  },
  {
    id: "ref_txn_3",
    type: "adjustment",
    amount: 15,
    currency: "USD",
    description: "Goodwill credit — delayed FTD attribution on Diego's account",
    createdAt: "2026-05-10T00:00:00Z",
    createdByMemberId: "mem_owner",
  },
  {
    id: "ref_txn_4",
    type: "payout_paid",
    amount: -75,
    currency: "USD",
    description: "Payout paid — bank transfer",
    relatedPayoutId: "ref_payout_1",
    createdAt: "2026-06-01T00:00:00Z",
    createdByMemberId: "mem_owner",
  },
];

export type PayoutStatus = "pending" | "approved" | "paid" | "rejected";

export type PayoutRequest = {
  id: string;
  amount: number;
  currency: "USD";
  status: PayoutStatus;
  requestedAt: string;
  requestedByMemberId: string;
  resolvedAt: string | null;
  resolvedByMemberId: string | null;
  note: string;
};

export const PAYOUT_REQUESTS: PayoutRequest[] = [
  {
    id: "ref_payout_1",
    amount: 75,
    currency: "USD",
    status: "paid",
    requestedAt: "2026-05-25T00:00:00Z",
    requestedByMemberId: "mem_owner",
    resolvedAt: "2026-06-01T00:00:00Z",
    resolvedByMemberId: "mem_owner",
    note: "",
  },
];

/** Derived, never stored — the whole point of keeping this as a ledger. */
export function computeBalances(transactions: ReferralTransaction[], payouts: PayoutRequest[]) {
  const totalEarned = transactions
    .filter((t) => t.type === "accrual" || (t.type === "adjustment" && t.amount > 0))
    .reduce((sum, t) => sum + t.amount, 0);
  const totalAdjustedDown = transactions
    .filter((t) => t.type === "adjustment" && t.amount < 0)
    .reduce((sum, t) => sum + t.amount, 0);
  const totalPaid = transactions.filter((t) => t.type === "payout_paid").reduce((sum, t) => sum + Math.abs(t.amount), 0);
  const pendingPayout = payouts
    .filter((p) => p.status === "pending" || p.status === "approved")
    .reduce((sum, p) => sum + p.amount, 0);
  const availableBalance = totalEarned + totalAdjustedDown - totalPaid - pendingPayout;

  return { totalEarned: totalEarned + totalAdjustedDown, totalPaid, pendingPayout, availableBalance };
}

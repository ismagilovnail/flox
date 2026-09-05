/**
 * Referral Program (§30.7) — real backend: apps/internal/referral. One
 * referral account per workspace (org-refers-org, team-scoped per
 * §36-TENANCY), not per individual member — the code/link and balance
 * belong to the tenant. Amounts are flat USD (FLOX's own commission to the
 * tenant, not tenant traffic revenue) — §50-FX's original-currency/
 * event-date normalization deliberately does not apply here. The balance
 * is never stored client-side either — it always comes straight from the
 * real GET /referral response, computed server-side from the append-only
 * ledger (§54 audit trail).
 *
 * Moved out of lib/mock/ — real, permanent vocabulary. Same move already
 * made for lib/tags.ts, lib/custom-metrics.ts, lib/organization.ts,
 * lib/report-presets.ts.
 */

export type SignupStatus = "trial" | "active_customer" | "churned";

export type ReferredSignup = {
  id: string;
  organizationId: string;
  referredOrganizationId: string;
  name: string;
  email: string;
  status: SignupStatus;
  signedUpAt: string;
};

export type ReferralTransactionType = "accrual" | "adjustment" | "payout_paid";

export type ReferralTransaction = {
  id: string;
  organizationId: string;
  type: ReferralTransactionType;
  /** Sign carries meaning: positive = credit, negative = debit. */
  amount: number;
  currency: "USD";
  description: string;
  relatedSignupId?: string;
  relatedPayoutId?: string;
  createdByUserId: string;
  createdAt: string;
};

export type PayoutStatus = "pending" | "approved" | "paid" | "rejected";

export type PayoutRequest = {
  id: string;
  organizationId: string;
  amount: number;
  currency: "USD";
  status: PayoutStatus;
  requestedAt: string;
  requestedByUserId: string;
  resolvedAt?: string;
  resolvedByUserId?: string;
  note: string;
};

export type ReferralBalance = {
  totalEarned: number;
  totalPaid: number;
  pendingPayout: number;
  availableBalance: number;
};

/** Hydrates the whole referral page in one call — code/link, balance, and
 * all three lists. */
export type ReferralDashboard = {
  code: string;
  link: string;
  balance: ReferralBalance;
  signups: ReferredSignup[];
  transactions: ReferralTransaction[];
  payouts: PayoutRequest[];
};

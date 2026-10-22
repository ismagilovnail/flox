import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/billing (BIZ-1, docs/billing.md). A `null` limit
 * is unlimited. */
export type Plan = {
  code: string;
  name: string;
  monthlyClicks: number | null;
  maxCampaigns: number | null;
  maxDomains: number | null;
  maxMembers: number | null;
  priceCents: number;
  currency: string;
  /** Monthly price in kopecks it is paid online for; null: not sold online. */
  priceRub: number | null;
};

/** How far this month's clicks are into the plan: warning from 80%,
 * exceeded from 100%, blocked from 110% (no new campaigns or stream sets).
 * unknown: the clicks could not be read. */
export type BillingLevel = "ok" | "warning" | "exceeded" | "blocked" | "unknown";

export type BillingStatus = {
  plan: Plan;
  since: string;
  /** UTC calendar month, both days inclusive. */
  period: { from: string; to: string };
  usage: { clicks: number | null; campaigns: number; domains: number; members: number };
  level: BillingLevel;
  /** End of the paid period; null when the plan is open-ended. */
  paidUntil: string | null;
  /** When the organization moves to Free unless it renews. */
  graceUntil: string | null;
  /** The paid period ends within 3 days, or has ended. */
  renew: boolean;
  /** Plans can be paid online (YooKassa) on this server. */
  canPay: boolean;
};

export type PaymentMonths = 1 | 3 | 12;

/** A plan payment (YooKassa). `confirmationUrl`: the payment page, while
 * the payment can still be paid. Amounts in kopecks. */
export type Payment = {
  id: string;
  planCode: string;
  months: number;
  amount: number;
  currency: string;
  status: "pending" | "succeeded" | "canceled";
  cancellationReason: string;
  confirmationUrl?: string;
  appliedAt: string | null;
  createdAt: string;
};

export function getBilling(): Promise<BillingStatus> {
  return apiFetch("/billing");
}

export function listPlans(): Promise<{ plans: Plan[] }> {
  return apiFetch("/billing/plans");
}

export function checkout(input: { plan: string; months: PaymentMonths }): Promise<Payment> {
  return apiFetch("/billing/checkout", { method: "POST", body: input });
}

export function listPayments(): Promise<{ payments: Payment[] }> {
  return apiFetch("/billing/payments");
}

export function getPayment(id: string): Promise<Payment> {
  return apiFetch(`/billing/payments/${encodeURIComponent(id)}`);
}

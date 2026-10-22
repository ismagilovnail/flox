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
};

export function getBilling(): Promise<BillingStatus> {
  return apiFetch("/billing");
}

export function listPlans(): Promise<{ plans: Plan[] }> {
  return apiFetch("/billing/plans");
}

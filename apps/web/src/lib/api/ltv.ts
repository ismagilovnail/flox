import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/ltv/handler.go. Cohorts are anchored on a click's
 * first deposit (FTD, CPA_ACCEPT) or registration (Reg, CPA_HOLD); every
 * number is computed by the Go engine — this side only displays it. */
export type CohortPeriod = "day" | "week" | "month";
export type CohortType = "ftd" | "reg";
/** §26.5 breakdowns; Go ltv.Dimension. None = one row per cohort period. */
export const LTV_DIMENSIONS = ["campaign", "source", "offer", "network", "country"] as const;
export type LtvDimension = (typeof LTV_DIMENSIONS)[number];
export const LTV_WINDOWS = ["d0", "d1_7", "d8_30", "d31_90"] as const;
export type LtvWindow = (typeof LTV_WINDOWS)[number];

export interface LtvWindowValue {
  revenueUsd: number;
  /** false while the window's days have not all passed for the cohort's
   * youngest member — the revenue is partial, not final. */
  complete: boolean;
}

export type Cohort = {
  bucket: string;
  campaignId?: string;
  sourceId?: string;
  offerId?: string;
  networkId?: string;
  country?: string;
  anchorCount: number;
  windows: Partial<Record<LtvWindow, LtvWindowValue>>;
  ltvTotalUsd: number;
  ltvPerAnchorUsd: number;
  lifetimeDaysAvg?: number;
  hasLifetimeData: boolean;
  redepUniqueCount: number;
  totalDeposits: number;
  totalDepositRevenueUsd: number;
  ftdToRedepRate?: number;
  depToRedepRate?: number;
  regToFtdRate?: number;
};

export interface CohortQuery {
  type: CohortType;
  period: CohortPeriod;
  from: string;
  to: string;
  campaignId?: string;
  sourceId?: string;
  offerId?: string;
  networkId?: string;
  country?: string;
  groupBy: LtvDimension[];
}

export function listCohorts(q: CohortQuery): Promise<{ cohorts: Cohort[]; currency: string }> {
  return apiFetch(`/analytics/ltv/${q.type}-cohorts`, {
    searchParams: {
      period: q.period,
      from: q.from,
      to: q.to,
      campaignId: q.campaignId,
      sourceId: q.sourceId,
      offerId: q.offerId,
      networkId: q.networkId,
      country: q.country,
      groupBy: q.groupBy.join(",") || undefined,
    },
  });
}

/**
 * Conversions list + detail/timeline (§29, §43) — the real API layer,
 * backed by apps/internal/conversions (a thin read layer over ClickHouse's
 * conversion_events/click_events/tracking_events, mirroring how
 * apps/internal/analytics already reads this store). Org-wide, not scoped
 * to one campaign — matches the top-level "Conversions" nav item.
 *
 * CpaStatus/CPA_STATUSES live here (not in lib/mock/conversions.ts) because
 * they're a real domain enum, not mock-specific — lib/mock/conversions.ts
 * and the still-mocked Postback Logs/Event Mappings features that depend on
 * it import the type from here instead of redeclaring it.
 */

import { apiFetch } from "@/lib/api/client";

export type CpaStatus = "CPA_HOLD" | "CPA_ACCEPT" | "CPA_REDEP" | "CPA_DECLINE" | "CPA_TRASH";
export const CPA_STATUSES: CpaStatus[] = ["CPA_HOLD", "CPA_ACCEPT", "CPA_REDEP", "CPA_DECLINE", "CPA_TRASH"];

/** i18n keys (conversions.json namespace) for each CPA status's display
 * label — the status code itself (CPA_HOLD, ...) stays untranslated
 * everywhere else (API payloads, cache keys, the §45 dedup key). */
export const CPA_STATUS_I18N_KEY: Record<CpaStatus, string> = {
  CPA_HOLD: "status.hold",
  CPA_ACCEPT: "status.accept",
  CPA_REDEP: "status.redep",
  CPA_DECLINE: "status.decline",
  CPA_TRASH: "status.trash",
};

/** The full §43 event model — a timeline entry can be any of these, not
 * just a CPA status. Matches apps/internal/event.Type's All exactly. */
export type EventType =
  | "SOURCE_CLICK"
  | "SOURCE_FILTER"
  | "LAND_VIEW"
  | "LAND_CLICK"
  | "POSTLANDING_VIEW"
  | "POSTLANDING_CLICK"
  | "PWA_VIEW"
  | "PWA_OPEN"
  | "PWA_INSTALL"
  | "IOS_INSTALL"
  | "NOTIFICATION_REQUEST"
  | "NOTIFICATION_SUBSCRIBE"
  | "NOTIFICATION_DECLINE"
  | "NOTIFICATION_UNSUBSCRIBE"
  | "NOTIFICATION_CLICK"
  | "TG_JOIN"
  | "TG_START"
  | CpaStatus;

/** One CPA_* row for the Conversions list. A click_id can appear more than
 * once (HOLD, then ACCEPT, then REDEP, ...) — real status history, not
 * duplicate rows to dedupe client-side. No offerId/campaign-name/
 * network-name: conversion_events carries no offer_id at all (only
 * flow_id, which would need a separate Postgres join to resolve an offer —
 * out of this phase's scope), and names are resolved client-side from the
 * real useCampaigns()/useNetworks() hooks, same pattern the list view
 * already used for its old mock id→name maps. */
export type Conversion = {
  eventAt: string;
  type: CpaStatus;
  campaignId: string;
  clickId: string;
  networkId: string;
  revenue: number;
  currency: string;
  usdValue: number;
  hasUsdValue: boolean;
};

export type ListConversionsResult = {
  conversions: Conversion[];
  total: number;
};

/** One entry in a click_id's real, variable-length funnel — whatever
 * stages actually happened, not a fixed six-item shape every conversion is
 * forced into. Conversion-only fields are absent when isConversion is
 * false. */
export type TimelineEvent = {
  eventAt: string;
  type: EventType;
  isConversion: boolean;
  revenue?: number;
  currency?: string;
  usdValue?: number;
  hasUsdValue?: boolean;
};

export type ClickTimeline = {
  clickId: string;
  campaignId: string;
  networkId?: string;
  events: TimelineEvent[];
};

export type ListConversionsParams = {
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export function listConversions(params: ListConversionsParams = {}): Promise<ListConversionsResult> {
  return apiFetch("/conversions", {
    searchParams: {
      from: params.from,
      to: params.to,
      limit: params.limit?.toString(),
      offset: params.offset?.toString(),
    },
  });
}

export function getConversionTimeline(clickId: string): Promise<ClickTimeline> {
  return apiFetch(`/conversions/${clickId}`);
}

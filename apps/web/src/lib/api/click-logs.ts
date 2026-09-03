/**
 * Click Log (§48) — the real API layer for apps/internal/clicklogs, a thin
 * read-only layer over ClickHouse's click_events (SOURCE_CLICK /
 * SOURCE_FILTER only — the tracker's own redirect-time decision, not the
 * later funnel events tracking_events holds). Org-wide, newest first, same
 * date-range/pagination contract as lib/api/postback-logs.ts. No writes —
 * a click_events row isn't a retriable action the way a postback attempt
 * is, so there's no replay counterpart here.
 */

import { apiFetch } from "@/lib/api/client";

export type ClickLogType = "SOURCE_CLICK" | "SOURCE_FILTER";

export type ClickLog = {
  eventAt: string;
  type: ClickLogType;

  campaignId: string;
  clickId: string;
  streamSetId?: string;
  flowId?: string;
  destination?: string;
  stickyApplied?: boolean;

  country?: string;
  region?: string;
  city?: string;
  device?: string;
  platform?: string;
  os?: string;
  osVersion?: string;
  browser?: string;
  browserVersion?: string;
  language?: string;
  isBot?: boolean;
  isProxy?: boolean;
  asn?: string;
  connectionType?: string;
  ip?: string;
  userAgent?: string;
  referrer?: string;

  utmSource?: string;
  utmMedium?: string;
  utmCampaign?: string;
  utmContent?: string;
  utmTerm?: string;

  sub1?: string;
  sub2?: string;
  sub3?: string;
  sub4?: string;
  sub5?: string;
  sub6?: string;
  sub7?: string;
  sub8?: string;
  sub9?: string;
  sub10?: string;

  externalClickId?: string;
  fbClickId?: string;
  ttClickId?: string;
  filterReason?: string;
};

export type ListClickLogsResult = {
  logs: ClickLog[];
  total: number;
};

export type ListClickLogsParams = {
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export function listClickLogs(params: ListClickLogsParams = {}): Promise<ListClickLogsResult> {
  return apiFetch("/click-log", {
    searchParams: {
      from: params.from,
      to: params.to,
      limit: params.limit?.toString(),
      offset: params.offset?.toString(),
    },
  });
}

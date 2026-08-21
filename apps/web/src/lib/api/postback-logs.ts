/**
 * Postback Logs (§29, §45) — the real API layer for
 * apps/internal/postbacklogs, a thin read layer over ClickHouse's
 * postback_events. Org-wide, both directions mixed in one list, matching
 * the frontend's single Logs table. Read-only: replay (re-invoking the
 * conversion engine for an incoming row, or re-enqueuing a delivery for
 * an outgoing one) was deliberately scoped out of this phase — see
 * docs/postback-logs.md.
 */

import { apiFetch } from "@/lib/api/client";
import type { CpaStatus } from "@/lib/api/conversions";

export type PostbackDirection = "incoming" | "outgoing";

/** The real result vocabulary is wider than the old mock's
 * success/duplicate/error: incoming is success/duplicate/ignored/error,
 * outgoing is queued/processing/success/failed/retrying/dead (§45/§46). */
export type PostbackResult =
  | "success"
  | "duplicate"
  | "ignored"
  | "error"
  | "queued"
  | "processing"
  | "retrying"
  | "dead"
  | "failed";

export type PostbackLog = {
  eventAt: string;
  direction: PostbackDirection;
  networkId: string;
  clickId: string;
  status?: CpaStatus;
  rawStatus?: string;
  result: PostbackResult;
  message: string;
  attemptCount?: number;
  responseStatusCode?: number;
  url?: string;
  revenue?: number;
  currency?: string;
};

export type ListPostbackLogsResult = {
  logs: PostbackLog[];
  total: number;
};

export type ListPostbackLogsParams = {
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export function listPostbackLogs(params: ListPostbackLogsParams = {}): Promise<ListPostbackLogsResult> {
  return apiFetch("/postback-logs", {
    searchParams: {
      from: params.from,
      to: params.to,
      limit: params.limit?.toString(),
      offset: params.offset?.toString(),
    },
  });
}

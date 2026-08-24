/**
 * Postback Logs (§29, §45/§46) — the real API layer for
 * apps/internal/postbacklogs, a thin layer mostly reading ClickHouse's
 * postback_events. Org-wide, both directions mixed in one list, matching
 * the frontend's single Logs table.
 *
 * `replayOutgoingPostback` is the one write this file does: it re-enqueues
 * a fresh delivery for a past outgoing attempt (the exact fields a
 * PostbackLog row already carries, no second fetch), through the same
 * path a first attempt already takes. Incoming replay (re-invoking the
 * conversion engine for an incoming row) is still deliberately deferred —
 * see docs/postback-logs.md for why the two turned out to be very
 * different sizes.
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
  eventRef?: string;
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

export type ReplayOutgoingPostbackInput = {
  networkId: string;
  clickId: string;
  status: CpaStatus;
  eventRef?: string;
  url: string;
};

export type ReplayOutgoingPostbackResult = {
  deliveryId: string;
};

/** Re-enqueues a fresh delivery for a past outgoing attempt — pass the
 * exact fields off the PostbackLog row being replayed, no re-derivation. */
export function replayOutgoingPostback(input: ReplayOutgoingPostbackInput): Promise<ReplayOutgoingPostbackResult> {
  return apiFetch("/postback-logs/replay-outgoing", { method: "POST", body: input });
}

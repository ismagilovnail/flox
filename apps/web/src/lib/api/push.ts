import { apiFetch } from "@/lib/api/client";
import { fetchAllPages } from "@/lib/api/pagination";

/** Mirrors apps/internal/push (docs/push.md). */
export type PushTarget = "destination" | "url";

export type PushFilters = {
  pwaIds: string[];
  campaignIds: string[];
  countries: string[];
};

export type PushBroadcast = {
  id: string;
  name: string;
  title: string;
  body: string;
  iconUrl: string;
  imageUrl: string;
  target: PushTarget;
  url: string;
  filters: PushFilters;
  total: number;
  delivered: number;
  failed: number;
  unsubscribed: number;
  opened: number;
  /** scheduled: waits for sendAt, no deliveries yet (the audience is fixed
   * when it goes out); sent: queued at sentAt; canceled: called off. */
  status: "scheduled" | "sent" | "canceled";
  sendAt: string | null;
  sentAt: string | null;
  createdAt: string;
};

export type PushGroupCount = { key: string; count: number };

export type PushSummary = {
  active: number;
  unsubscribed: number;
  byPwa: PushGroupCount[];
  byCampaign: PushGroupCount[];
  byCountry: PushGroupCount[];
};

export type PushSubscription = {
  id: string;
  pwaId: string;
  campaignId: string;
  clickId: string;
  pushService: string;
  country: string;
  device: string;
  language: string;
  status: "active" | "unsubscribed";
  unsubscribedAt?: string;
  createdAt: string;
};

export type CreateBroadcastInput = {
  name: string;
  title: string;
  body: string;
  iconUrl: string;
  imageUrl: string;
  target: PushTarget;
  url: string;
  filters: PushFilters;
  /** RFC 3339; absent sends now. A minute to 30 days ahead. */
  sendAt?: string;
};

/** Push metrics keyed as the custom-metric catalog names them (§30.5,
 * data source "push"), so a push formula evaluates on a row as is.
 * push_ctr = opened ÷ delivered × 100, null when nothing was delivered. */
export type PushMetrics = {
  push_sent: number;
  push_delivered: number;
  push_opened: number;
  push_ctr: number | null;
  push_failed: number;
  push_unsubscribed: number;
};

export type PushStatsGroup = "day" | "broadcast" | "pwa" | "campaign" | "country";
export const PUSH_STATS_GROUPS: PushStatsGroup[] = ["day", "broadcast", "pwa", "campaign", "country"];

/** A row's key is the day (YYYY-MM-DD in `timezone`), broadcast, PWA or
 * campaign id, or country; `label` is the broadcast's name. Broadcasts
 * count on their send day. */
export type PushStats = {
  rows: { key: string; label?: string; metrics: PushMetrics }[];
  totals: PushMetrics;
};

/** `configured`: the deployment has VAPID keys — without them nothing can be sent. */
export function getPushSummary(): Promise<{ summary: PushSummary; configured: boolean }> {
  return apiFetch("/push/summary");
}

export function listPushSubscriptions(): Promise<{ subscriptions: PushSubscription[]; total: number }> {
  return fetchAllPages<"subscriptions", PushSubscription>("/push/subscriptions", "subscriptions");
}

export function countPushAudience(filters: PushFilters): Promise<{ count: number }> {
  return apiFetch("/push/audience", { method: "POST", body: filters });
}

export function listPushBroadcasts(): Promise<{ broadcasts: PushBroadcast[]; total: number }> {
  return fetchAllPages<"broadcasts", PushBroadcast>("/push/broadcasts", "broadcasts");
}

export function createPushBroadcast(input: CreateBroadcastInput): Promise<PushBroadcast> {
  return apiFetch("/push/broadcasts", { method: "POST", body: input });
}

export function cancelPushBroadcast(id: string): Promise<PushBroadcast> {
  return apiFetch(`/push/broadcasts/${id}/cancel`, { method: "POST" });
}

export function getPushStats(input: { from: string; to: string; timezone: string; groupBy: PushStatsGroup }): Promise<PushStats> {
  return apiFetch("/push/stats", { searchParams: input });
}

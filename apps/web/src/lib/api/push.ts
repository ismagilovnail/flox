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

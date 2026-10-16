/** A visitor's data request (internal/visitordata, audit GDPR-1): what FLOX
 * holds about one visitor, found by click id or IP, and its erasure. */

import { apiFetch } from "@/lib/api/client";

export type VisitorReport = {
  clickIds: string[];
  truncated: boolean;
  clicks: {
    at: string;
    type: string;
    clickId: string;
    campaignId: string;
    country: string;
    city: string;
    device: string;
    os: string;
    browser: string;
    ip: string;
    userAgent: string;
    referrer: string;
  }[];
  events: { at: string; type: string; clickId: string }[];
  conversions: { at: string; type: string; clickId: string; revenue: number; currency: string }[];
  pushSubscriptions: { clickId: string; createdAt: string; status: string; country: string; device: string; language: string; endpointHost: string }[];
  telegramUsers: { clickId: string; tgUserId: number; startedAt: string }[];
  pixelDeliveries: number;
};

export type VisitorErased = {
  pushSubscriptions: number;
  telegramUsers: number;
  telegramJoins: number;
  telegramInvites: number;
  pixelDeliveries: number;
};

export function findVisitor(query: { clickId?: string; ip?: string }): Promise<VisitorReport> {
  const params = new URLSearchParams();
  if (query.clickId) params.set("clickId", query.clickId);
  if (query.ip) params.set("ip", query.ip);
  return apiFetch(`/privacy/visitor?${params.toString()}`);
}

export function eraseVisitor(clickIds: string[]): Promise<VisitorErased> {
  return apiFetch("/privacy/visitor/erase", { method: "POST", body: { clickIds } });
}

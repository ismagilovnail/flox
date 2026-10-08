/** Alerts (marketer audit #10). Contract: apps/internal/alert — rules are
 * evaluated by the worker; this side only edits rules and shows what fired. */

export const ALERT_KINDS = ["spend_over", "roi_below", "offer_capped", "offer_paused", "ssl_expiring"] as const;
export type AlertKind = (typeof ALERT_KINDS)[number];

export interface AlertRule {
  id: string;
  kind: AlertKind;
  /** spend_over / roi_below only; null checks every campaign separately. */
  campaignId: string | null;
  /** USD for spend_over, percent for roi_below, days for ssl_expiring. */
  threshold: number | null;
  /** roi_below only: below this spend ROI is too early to judge. */
  minSpend: number | null;
  notifySlack: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface AlertRuleInput {
  kind: AlertKind;
  campaignId: string | null;
  threshold: number | null;
  minSpend: number | null;
  notifySlack: boolean;
  enabled: boolean;
}

export interface AppNotification {
  id: string;
  ruleId: string | null;
  kind: AlertKind;
  params: Record<string, string | number | boolean | null>;
  createdAt: string;
  read: boolean;
}

export interface NotificationList {
  notifications: AppNotification[];
  unread: number;
}

/** Which kinds take a campaign scope and which a threshold. */
export function kindUsesCampaign(kind: AlertKind): boolean {
  return kind === "spend_over" || kind === "roi_below";
}

export function kindUsesThreshold(kind: AlertKind): boolean {
  return kind === "spend_over" || kind === "roi_below" || kind === "ssl_expiring";
}

/** Where a notification leads when clicked. */
export function notificationHref(n: AppNotification): string | null {
  const p = n.params;
  switch (n.kind) {
    case "spend_over":
    case "roi_below":
      return typeof p.campaignId === "string" ? `/campaigns/${p.campaignId}` : null;
    case "offer_capped":
    case "offer_paused":
      return typeof p.offerId === "string" ? `/offers/${p.offerId}` : null;
    case "ssl_expiring":
      return "/domains";
  }
}

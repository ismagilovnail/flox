import { describe, expect, it } from "vitest";

import i18next from "@/lib/i18n/config";
import { notificationText } from "@/features/alerts/notification-text";
import { kindUsesCampaign, kindUsesThreshold, notificationHref, type AppNotification } from "@/lib/alerts";

function n(kind: AppNotification["kind"], params: AppNotification["params"]): AppNotification {
  return { id: "1", ruleId: null, kind, params, createdAt: "2026-10-08T10:00:00Z", read: false };
}

describe("notificationText", () => {
  const ru = i18next.getFixedT("ru", "alerts");
  const en = i18next.getFixedT("en", "alerts");

  it("renders spend and ROI with the viewer's number format", () => {
    const spend = notificationText(ru, "ru", n("spend_over", { campaignName: "FB US", spend: 1520.5, threshold: 1000 }));
    expect(spend.title).toBe("Расход выше порога");
    expect(spend.body).toBe("FB US: расход за сегодня $1 520,5, порог $1 000.");
    const roi = notificationText(en, "en", n("roi_below", { campaignName: "FB US", roi: -58.33, threshold: 0, spend: 120, revenue: 50 }));
    expect(roi.body).toBe("FB US: ROI -58.33% today, below 0% (spend $120, revenue $50).");
  });

  it("pluralizes certificate days and tells expired apart", () => {
    const params = { domain: "t.example.com", daysLeft: 5, expiresAt: "2026-10-13T10:00:00Z", expired: false };
    expect(notificationText(ru, "ru", n("ssl_expiring", params)).body).toMatch(/^t\.example\.com: сертификат истекает через 5 дней \(/);
    expect(notificationText(ru, "ru", n("ssl_expiring", { ...params, daysLeft: 2 })).body).toMatch(/через 2 дня/);
    expect(notificationText(en, "en", n("ssl_expiring", { ...params, daysLeft: 1 })).body).toMatch(/in 1 day \(/);
    expect(notificationText(en, "en", n("ssl_expiring", { ...params, expired: true })).title).toBe("SSL certificate expired");
  });

  it("renders offer cap progress", () => {
    const capped = notificationText(ru, "ru", n("offer_capped", { offerName: "Offer One", count: 100, cap: 100 }));
    expect(capped.body).toBe("Offer One: 100 из 100 конверсий — трафик не идёт до полуночи.");
  });
});

describe("alert helpers", () => {
  it("links each notification to where it is fixed", () => {
    expect(notificationHref(n("spend_over", { campaignId: "C1" }))).toBe("/campaigns/C1");
    expect(notificationHref(n("offer_paused", { offerId: "O1" }))).toBe("/offers/O1");
    expect(notificationHref(n("ssl_expiring", {}))).toBe("/domains");
    expect(notificationHref(n("roi_below", {}))).toBeNull();
  });

  it("knows which kinds take a campaign and a threshold", () => {
    expect(kindUsesCampaign("roi_below")).toBe(true);
    expect(kindUsesCampaign("ssl_expiring")).toBe(false);
    expect(kindUsesThreshold("ssl_expiring")).toBe(true);
    expect(kindUsesThreshold("offer_paused")).toBe(false);
  });
});

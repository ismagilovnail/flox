"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { useLocale } from "@/hooks/use-locale";
import type { AppNotification } from "@/lib/alerts";

export function useNotificationText() {
  const { t } = useTranslation("alerts");
  const locale = useLocale();
  return (n: AppNotification) => notificationText(t, locale, n);
}

/** Renders a notification's params as a title and body in the viewer's
 * language — the server stores the numbers and names, not the text. */
export function notificationText(t: TFunction<"alerts">, locale: string, n: AppNotification): { title: string; body: string } {
  const num = new Intl.NumberFormat(locale, { maximumFractionDigits: 2 });
  const p = n.params;
  const str = (key: string) => (typeof p[key] === "string" ? (p[key] as string) : "");
  const amount = (key: string) => (typeof p[key] === "number" ? num.format(p[key] as number) : "—");
  switch (n.kind) {
    case "spend_over":
      return {
        title: t("messages.spend_over.title"),
        body: t("messages.spend_over.body", { campaign: str("campaignName"), spend: amount("spend"), threshold: amount("threshold") }),
      };
    case "roi_below":
      return {
        title: t("messages.roi_below.title"),
        body: t("messages.roi_below.body", {
          campaign: str("campaignName"),
          roi: amount("roi"),
          threshold: amount("threshold"),
          spend: amount("spend"),
          revenue: amount("revenue"),
        }),
      };
    case "offer_capped":
      return {
        title: t("messages.offer_capped.title"),
        body: t("messages.offer_capped.body", { offer: str("offerName"), used: amount("count"), cap: amount("cap") }),
      };
    case "offer_paused":
      return { title: t("messages.offer_paused.title"), body: t("messages.offer_paused.body", { offer: str("offerName") }) };
    case "ssl_expiring": {
      if (p.expired === true) {
        return { title: t("messages.ssl_expired.title"), body: t("messages.ssl_expired.body", { domain: str("domain") }) };
      }
      const expires = str("expiresAt");
      const date = expires ? new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(new Date(expires)) : "";
      const count = typeof p.daysLeft === "number" ? p.daysLeft : 0;
      return {
        title: t("messages.ssl_expiring.title"),
        body: t("messages.ssl_expiring.body", { domain: str("domain"), count, date }),
      };
    }
  }
}

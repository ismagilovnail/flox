"use client";

import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { useCampaignDiagnostics } from "@/hooks/use-campaigns";

/** Clicks the tracker turned away for this campaign: over the per-IP limit
 * they go to the fallback and are in no report (audit SEC-4, REL-2).
 * Silent when there were none or the diagnostics cannot be read: it is
 * advice, not the page. */
export function CampaignDiagnostics({ campaignId }: { campaignId: string }) {
  const { t, i18n } = useTranslation("campaigns");
  const { data } = useCampaignDiagnostics(campaignId);
  const limited = data?.ipLimited;
  if (!limited || limited.clicks === 0) return null;

  const lastDay = limited.lastDay
    ? new Date(limited.lastDay).toLocaleDateString(i18n.language, { day: "numeric", month: "long", timeZone: "UTC" })
    : "";
  return (
    <Alert className="border-warning/40">
      <TriangleAlert aria-hidden="true" className="text-warning" />
      <AlertTitle>{t("diagnostics.ipLimited.title", { count: limited.clicks, days: limited.windowDays })}</AlertTitle>
      <AlertDescription>
        <p>{t("diagnostics.ipLimited.description", { lastDay })}</p>
        <p>{t("diagnostics.ipLimited.fix")}</p>
      </AlertDescription>
    </Alert>
  );
}

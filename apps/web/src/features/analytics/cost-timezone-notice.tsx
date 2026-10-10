"use client";

import { useTranslation } from "react-i18next";
import { Clock } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { useCostTimezones } from "@/hooks/use-analytics-report";

/**
 * Says when synced spend in the range is daily in an ad account's own time
 * zone and that zone's offset differs from the report's (tech audit AN4).
 * Renders nothing otherwise — while loading, on error, or when they match:
 * it is a note about the numbers, never a reason to hold the report back.
 */
export function CostTimezoneNotice({ from, to, timezone }: { from: Date; to: Date; timezone?: string }) {
  const { t } = useTranslation("cost");
  const query = useCostTimezones(from, to, timezone);
  const zones = query.data ?? [];
  if (zones.length === 0) return null;
  return (
    <Alert>
      <Clock aria-hidden="true" />
      <AlertTitle>{t("timezoneNotice.title")}</AlertTitle>
      <AlertDescription>
        {t("timezoneNotice.description", { zones: zones.join(", "), timezone: timezone || "UTC" })}
      </AlertDescription>
    </Alert>
  );
}

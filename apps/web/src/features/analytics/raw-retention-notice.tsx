"use client";

import { useTranslation } from "react-i18next";
import { History } from "lucide-react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { useBilling } from "@/hooks/use-billing";

/** The first moment raw clicks are still kept: `months` before `now`. */
export function rawRetentionCutoff(months: number, now: Date = new Date()): Date {
  const cutoff = new Date(now);
  cutoff.setUTCMonth(cutoff.getUTCMonth() - months);
  return cutoff;
}

/**
 * Says when the range reaches further back than the organization keeps raw
 * clicks (audit DATA-6, docs/retention.md). Reports by the main dimensions
 * come from the hourly rollup and are whole; what needs single clicks —
 * the click log, the funnel, region, city — is empty before the cutoff.
 * Renders nothing while the plan loads, or when the range is within it.
 */
export function RawRetentionNotice({ from, variant, now }: { from: Date; variant: "report" | "clickLog"; now?: Date }) {
  const { t } = useTranslation("analytics");
  const billing = useBilling();
  const months = billing.data?.rawRetentionMonths;
  if (!months || from >= rawRetentionCutoff(months, now)) return null;
  return (
    <Alert>
      <History aria-hidden="true" />
      <AlertTitle>{t("rawRetention.title", { count: months })}</AlertTitle>
      <AlertDescription>{t(`rawRetention.${variant}`, { count: months })}</AlertDescription>
    </Alert>
  );
}

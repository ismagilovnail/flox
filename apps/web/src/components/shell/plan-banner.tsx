"use client";

import Link from "next/link";
import { useTranslation } from "react-i18next";
import { CalendarClock, Gauge } from "lucide-react";

import { useBilling } from "@/hooks/use-billing";
import { formatDeletionDate } from "@/lib/format";
import { cn } from "@/lib/utils";

/** On every page (BIZ-1): when the paid period ends within 3 days or is in
 * its grace period, and once this month's clicks reach 80% of the plan.
 * Tracking never stops; from 110% new campaigns and stream sets wait. */
export function PlanBanner() {
  const { t, i18n } = useTranslation("settings");
  const billing = useBilling();
  const data = billing.data;
  if (!data) return null;
  const level = data.level;
  const usage = level === "warning" || level === "exceeded" || level === "blocked";
  const renew = data.renew && data.graceUntil;
  if (!usage && !renew) return null;

  return (
    <>
      {renew && (
        <div role="alert" className="flex flex-wrap items-center gap-3 border-b border-warning/40 bg-warning/10 px-6 py-2 text-sm">
          <CalendarClock aria-hidden="true" className="size-4 text-warning" />
          <span>
            {t("billing.banner.renew", {
              plan: data.plan.name,
              paidUntil: formatDeletionDate(data.paidUntil as string, i18n.language),
              graceUntil: formatDeletionDate(data.graceUntil as string, i18n.language),
            })}
          </span>
          <Link href="/settings?tab=billing" className="font-medium text-primary underline-offset-4 hover:underline">
            {t("billing.banner.renewLink")}
          </Link>
        </div>
      )}
      {usage && (
        <div
          role={level === "warning" ? "status" : "alert"}
          className={cn(
            "flex flex-wrap items-center gap-3 border-b px-6 py-2 text-sm",
            level === "warning" ? "border-warning/40 bg-warning/10" : "border-danger/40 bg-danger/10",
          )}
        >
          <Gauge aria-hidden="true" className={cn("size-4", level === "warning" ? "text-warning" : "text-danger")} />
          <span>{t(`billing.banner.${level}`, { plan: data.plan.name })}</span>
          <Link href="/settings?tab=billing" className="font-medium text-primary underline-offset-4 hover:underline">
            {t("billing.banner.details")}
          </Link>
        </div>
      )}
    </>
  );
}

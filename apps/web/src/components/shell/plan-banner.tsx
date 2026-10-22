"use client";

import Link from "next/link";
import { useTranslation } from "react-i18next";
import { Gauge } from "lucide-react";

import { useBilling } from "@/hooks/use-billing";
import { cn } from "@/lib/utils";

/** On every page once this month's clicks reach 80% of the plan (BIZ-1).
 * Tracking never stops; from 110% new campaigns and stream sets wait. */
export function PlanBanner() {
  const { t } = useTranslation("settings");
  const billing = useBilling();
  const level = billing.data?.level;
  if (level !== "warning" && level !== "exceeded" && level !== "blocked") return null;
  const danger = level !== "warning";

  return (
    <div
      role={danger ? "alert" : "status"}
      className={cn(
        "flex flex-wrap items-center gap-3 border-b px-6 py-2 text-sm",
        danger ? "border-danger/40 bg-danger/10" : "border-warning/40 bg-warning/10",
      )}
    >
      <Gauge aria-hidden="true" className={cn("size-4", danger ? "text-danger" : "text-warning")} />
      <span>{t(`billing.banner.${level}`, { plan: billing.data?.plan.name })}</span>
      <Link href="/settings?tab=billing" className="font-medium text-primary underline-offset-4 hover:underline">
        {t("billing.banner.details")}
      </Link>
    </div>
  );
}

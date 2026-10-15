"use client";

import Link from "next/link";
import { TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

/** Conversions in the range whose revenue has no USD value (no currency and
 * no network currency to assume, or no FX rate for that day) — their money
 * is missing from Revenue, Profit and ROI above. Says so instead of letting
 * the totals read quietly low (audit 2026-10-15, DATA-1). */
export function UnpricedRevenueNotice({ count }: { count: number }) {
  const { t } = useTranslation("dashboard");
  if (count <= 0) return null;
  return (
    <Alert className="border-warning/40">
      <TriangleAlert aria-hidden="true" className="text-warning" />
      <AlertTitle>{t("unpriced.title", { count })}</AlertTitle>
      <AlertDescription>
        <p>{t("unpriced.description")}</p>
        <Link href="/postbacks" className="font-medium text-primary underline-offset-4 hover:underline">
          {t("unpriced.link")}
        </Link>
      </AlertDescription>
    </Alert>
  );
}

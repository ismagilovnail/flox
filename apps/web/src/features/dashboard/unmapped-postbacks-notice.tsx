"use client";

import Link from "next/link";
import { ClockIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { useWaitingStatuses } from "@/hooks/use-event-mappings";

/** How many network statuses the notice names before "…". */
const NAMED_STATUSES = 5;

/** Postbacks whose network status no event mapping knows yet. The network
 * was answered 200 and will not send them again; they wait to be recorded
 * until the status is mapped, and are missing from the reports until then
 * (audit 2026-10-15, DATA-4). */
export function UnmappedPostbacksNotice() {
  const { t } = useTranslation("dashboard");
  const { data } = useWaitingStatuses();
  const statuses = data?.statuses ?? [];
  const count = statuses.reduce((sum, s) => sum + s.count, 0);
  if (count <= 0) return null;

  const names = [...new Set(statuses.map((s) => s.rawStatus))];
  const named = names.slice(0, NAMED_STATUSES).join(", ") + (names.length > NAMED_STATUSES ? ", …" : "");
  return (
    <Alert className="border-warning/40">
      <ClockIcon aria-hidden="true" className="text-warning" />
      <AlertTitle>{t("unmapped.title", { count })}</AlertTitle>
      <AlertDescription>
        <p>{t("unmapped.description", { statuses: named })}</p>
        <Link href="/postbacks?tab=mapping" className="font-medium text-primary underline-offset-4 hover:underline">
          {t("unmapped.link")}
        </Link>
      </AlertDescription>
    </Alert>
  );
}

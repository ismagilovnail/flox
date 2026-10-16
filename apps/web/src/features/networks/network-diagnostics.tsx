"use client";

import { Info, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { useNetworkDiagnostics } from "@/hooks/use-networks";

/** What the network's own postbacks say about its setup. Today one thing:
 * redeposits without txn_id, whose repeats are refused as duplicates — lost
 * revenue and LTV (audit 2026-10-15, DATA-2). The dedup key stays as §45
 * defines it; the fix is on the network's side. Silent when all is well or
 * the diagnostics cannot be read: it is advice, not the page. */
export function NetworkDiagnostics({ networkId }: { networkId: string }) {
  const { t } = useTranslation("networks");
  const { data } = useNetworkDiagnostics(networkId);
  const redep = data?.redepWithoutTxn;
  if (!data || !redep || redep.received === 0) return null;

  const lost = redep.duplicates > 0;
  const values = { days: data.windowDays, received: redep.received };
  return (
    <Alert className={lost ? "border-warning/40" : undefined}>
      {lost ? <TriangleAlert aria-hidden="true" className="text-warning" /> : <Info aria-hidden="true" />}
      <AlertTitle>
        {lost
          ? t("diagnostics.redepWithoutTxn.lostTitle", { count: redep.duplicates })
          : t("diagnostics.redepWithoutTxn.riskTitle")}
      </AlertTitle>
      <AlertDescription>
        <p>
          {lost
            ? t("diagnostics.redepWithoutTxn.lostDescription", values)
            : t("diagnostics.redepWithoutTxn.riskDescription", values)}
        </p>
        <p>{t("diagnostics.redepWithoutTxn.fix")}</p>
      </AlertDescription>
    </Alert>
  );
}

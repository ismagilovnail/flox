import { useTranslation } from "react-i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { NetworkList } from "@/features/networks/network-list";

/** Outgoing postbacks ARE the Network entity's postbackUrl/acceptDuplicates fields
 * (§27, extended §45) — reusing NetworkList here instead of a second table keeps
 * one source of truth for network config rather than duplicating the CRUD. */
export function OutgoingPostbacksPanel() {
  const { t } = useTranslation("postbacks");
  return (
    <div className="flex flex-col gap-4">
      <Alert>
        <AlertDescription>{t("outgoing.description")}</AlertDescription>
      </Alert>
      <NetworkList />
    </div>
  );
}

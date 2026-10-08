"use client";

"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { RelativeTime } from "@/components/ui/relative-time";
import type { ReferredSignup, SignupStatus } from "@/lib/referral";

const STATUS_VARIANT: Record<SignupStatus, "warning" | "success" | "secondary"> = {
  trial: "warning",
  active_customer: "success",
  churned: "secondary",
};

export function ReferredSignupsTable({ signups }: { signups: ReferredSignup[] }) {
  const { t } = useTranslation("referral");
  const columns: ColumnDef<typeof dataTableFeatures, ReferredSignup>[] = React.useMemo(
    () => [
      { accessorKey: "name", header: t("signups.name") },
      { accessorKey: "email", header: t("signups.email") },
      {
        accessorKey: "status",
        header: t("signups.status"),
        cell: ({ getValue }) => {
          const status = getValue() as SignupStatus;
          return <Badge variant={STATUS_VARIANT[status]}>{t(`signups.statuses.${status}`)}</Badge>;
        },
      },
      {
        accessorKey: "signedUpAt",
        header: t("signups.signedUp"),
        cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
      },
    ],
    [t],
  );
  return (
    <DataTable
      columns={columns}
      data={signups}
      searchPlaceholder={t("signups.search")}
      exportFilename="referral-signups"
      emptyTitle={t("signups.emptyTitle")}
      emptyDescription={t("signups.emptyDescription")}
      pageSize={10}
    />
  );
}

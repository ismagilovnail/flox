"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { formatUsd } from "@/lib/format";
import { PayoutRowActions } from "@/features/referral/payout-row-actions";
import type { PayoutRequest, PayoutStatus } from "@/lib/referral";

const STATUS_VARIANT: Record<PayoutStatus, "warning" | "info" | "success" | "danger"> = {
  pending: "warning",
  approved: "info",
  paid: "success",
  rejected: "danger",
};

export function PayoutsTable({ payouts, canManage }: { payouts: PayoutRequest[]; canManage: boolean }) {
  const { t } = useTranslation("referral");
  const columns: ColumnDef<typeof dataTableFeatures, PayoutRequest>[] = React.useMemo(
    () => [
      {
        accessorKey: "amount",
        header: t("payouts.amount"),
        cell: ({ getValue }) => <Mono>{formatUsd(getValue() as number, 2)}</Mono>,
      },
      {
        accessorKey: "status",
        header: t("payouts.status"),
        cell: ({ getValue }) => {
          const status = getValue() as PayoutStatus;
          return <Badge variant={STATUS_VARIANT[status]}>{t(`payouts.statuses.${status}`)}</Badge>;
        },
      },
      {
        accessorKey: "requestedAt",
        header: t("payouts.requested"),
        cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
      },
      {
        accessorKey: "note",
        header: t("payouts.note"),
        cell: ({ getValue }) => <Caption className="text-muted-foreground">{(getValue() as string) || "—"}</Caption>,
      },
      ...(canManage
        ? [
            {
              id: "actions",
              header: "",
              enableSorting: false,
              enableHiding: false,
              cell: ({ row }: { row: { original: PayoutRequest } }) => <PayoutRowActions payout={row.original} />,
            } satisfies ColumnDef<typeof dataTableFeatures, PayoutRequest>,
          ]
        : []),
    ],
    [canManage, t],
  );

  return (
    <DataTable
      columns={columns}
      data={payouts}
      searchPlaceholder={t("payouts.search")}
      exportFilename="referral-payouts"
      emptyTitle={t("payouts.emptyTitle")}
      emptyDescription={t("payouts.emptyDescription")}
      pageSize={10}
    />
  );
}

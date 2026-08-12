"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { formatUsd } from "@/lib/format";
import { useReferralStore } from "@/stores/referral";
import { PayoutRowActions } from "@/features/referral/payout-row-actions";
import type { PayoutRequest, PayoutStatus } from "@/lib/mock/referral";

const STATUS_VARIANT: Record<PayoutStatus, "warning" | "info" | "success" | "danger"> = {
  pending: "warning",
  approved: "info",
  paid: "success",
  rejected: "danger",
};

export function PayoutsTable({ canManage, currentMemberId }: { canManage: boolean; currentMemberId: string }) {
  const payouts = useReferralStore((s) => s.payouts);

  const columns: ColumnDef<typeof dataTableFeatures, PayoutRequest>[] = React.useMemo(
    () => [
      {
        accessorKey: "amount",
        header: "Amount",
        cell: ({ getValue }) => <Mono>{formatUsd(getValue() as number, 2)}</Mono>,
      },
      {
        accessorKey: "status",
        header: "Status",
        cell: ({ getValue }) => {
          const status = getValue() as PayoutStatus;
          return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
        },
      },
      {
        accessorKey: "requestedAt",
        header: "Requested",
        cell: ({ getValue }) => (
          <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>
        ),
      },
      {
        accessorKey: "note",
        header: "Note",
        cell: ({ getValue }) => <Caption className="text-muted-foreground">{(getValue() as string) || "—"}</Caption>,
      },
      ...(canManage
        ? [
            {
              id: "actions",
              header: "",
              enableSorting: false,
              enableHiding: false,
              cell: ({ row }: { row: { original: PayoutRequest } }) => (
                <PayoutRowActions payout={row.original} currentMemberId={currentMemberId} />
              ),
            } satisfies ColumnDef<typeof dataTableFeatures, PayoutRequest>,
          ]
        : []),
    ],
    [canManage, currentMemberId],
  );

  return (
    <DataTable
      columns={columns}
      data={payouts}
      searchPlaceholder="Search payouts..."
      emptyTitle="No payout requests yet"
      emptyDescription="Request a payout from your available balance above."
      pageSize={10}
    />
  );
}

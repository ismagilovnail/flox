"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { formatUsd } from "@/lib/format";
import { useTeamMembers } from "@/hooks/use-team";
import type { ReferralTransaction, ReferralTransactionType } from "@/lib/referral";

const TYPE_VARIANT: Record<ReferralTransactionType, "success" | "outline" | "warning"> = {
  accrual: "success",
  adjustment: "outline",
  payout_paid: "warning",
};

const TYPE_LABEL: Record<ReferralTransactionType, string> = {
  accrual: "Accrual",
  adjustment: "Adjustment",
  payout_paid: "Payout paid",
};

export function EarningsHistoryTable({ transactions }: { transactions: ReferralTransaction[] }) {
  // Real team roster (§28) — a transaction's createdByUserId is a real
  // users.id, resolved to a display name the same way the real team page
  // itself does. A creator who has since left the team falls back to the
  // raw id rather than hiding the row.
  const membersQuery = useTeamMembers();
  const memberNameByUserId = React.useMemo(
    () => Object.fromEntries((membersQuery.data ?? []).map((m) => [m.userId, m.name])),
    [membersQuery.data],
  );

  const columns: ColumnDef<typeof dataTableFeatures, ReferralTransaction>[] = React.useMemo(
    () => [
      {
        accessorKey: "type",
        header: "Type",
        cell: ({ getValue }) => {
          const type = getValue() as ReferralTransactionType;
          return <Badge variant={TYPE_VARIANT[type]}>{TYPE_LABEL[type]}</Badge>;
        },
      },
      {
        accessorKey: "amount",
        header: "Amount",
        cell: ({ getValue }) => {
          const amount = getValue() as number;
          return (
            <Mono className={amount >= 0 ? "text-success" : "text-danger"}>
              {amount >= 0 ? "+" : ""}
              {formatUsd(amount, 2)}
            </Mono>
          );
        },
      },
      { accessorKey: "description", header: "Description" },
      {
        id: "by",
        header: "By",
        accessorFn: (row) => memberNameByUserId[row.createdByUserId] ?? row.createdByUserId,
      },
      {
        accessorKey: "createdAt",
        header: "Date",
        cell: ({ getValue }) => (
          <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>
        ),
      },
    ],
    [memberNameByUserId],
  );

  return (
    <DataTable
      columns={columns}
      data={transactions}
      searchPlaceholder="Search history..."
      emptyTitle="No earnings yet"
      emptyDescription="Accruals, adjustments, and payouts appear here — an immutable, append-only ledger (§54)."
      pageSize={10}
    />
  );
}

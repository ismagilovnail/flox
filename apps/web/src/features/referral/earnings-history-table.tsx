"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { formatUsd } from "@/lib/format";
import { useReferralStore } from "@/stores/referral";
import { useTeamStore } from "@/stores/team";
import type { ReferralTransaction, ReferralTransactionType } from "@/lib/mock/referral";

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

export function EarningsHistoryTable() {
  const transactions = useReferralStore((s) => s.transactions);
  const members = useTeamStore((s) => s.members);

  const memberNameById = React.useMemo(() => Object.fromEntries(members.map((m) => [m.id, m.name])), [members]);

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
        accessorFn: (row) => memberNameById[row.createdByMemberId] ?? row.createdByMemberId,
      },
      {
        accessorKey: "createdAt",
        header: "Date",
        cell: ({ getValue }) => (
          <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>
        ),
      },
    ],
    [memberNameById],
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

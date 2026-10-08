"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { formatUsd } from "@/lib/format";
import { useTeamMembers } from "@/hooks/use-team";
import type { ReferralTransaction, ReferralTransactionType } from "@/lib/referral";

const TYPE_VARIANT: Record<ReferralTransactionType, "success" | "outline" | "warning"> = {
  accrual: "success",
  adjustment: "outline",
  payout_paid: "warning",
};


export function EarningsHistoryTable({ transactions }: { transactions: ReferralTransaction[] }) {
  // Real team roster (§28) — a transaction's createdByUserId is a real
  // users.id, resolved to a display name the same way the real team page
  // itself does. A creator who has since left the team falls back to the
  // raw id rather than hiding the row.
  const { t } = useTranslation("referral");
  const membersQuery = useTeamMembers();
  const memberNameByUserId = React.useMemo(
    () => Object.fromEntries((membersQuery.data ?? []).map((m) => [m.userId, m.name])),
    [membersQuery.data],
  );

  const columns: ColumnDef<typeof dataTableFeatures, ReferralTransaction>[] = React.useMemo(
    () => [
      {
        accessorKey: "type",
        header: t("history.type"),
        cell: ({ getValue }) => {
          const type = getValue() as ReferralTransactionType;
          return <Badge variant={TYPE_VARIANT[type]}>{t(`history.types.${type}`)}</Badge>;
        },
      },
      {
        accessorKey: "amount",
        header: t("history.amount"),
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
      { accessorKey: "description", header: t("history.description") },
      {
        id: "by",
        header: t("history.by"),
        // A `cell` renderer, not `accessorFn` — TanStack Table caches a row's
        // computed value per (row, columnId) the first time it's read, and
        // only invalidates that cache when the `data` array itself changes
        // reference, not when an accessorFn's closed-over external state
        // (memberNameByUserId, populated asynchronously after the team query
        // resolves) changes. An accessorFn here would silently freeze on
        // whatever it first resolved to — the raw id, since the team query
        // hasn't loaded on first render — even after the real name becomes
        // available. `cell` runs fresh on every render, so it always reads
        // the current memberNameByUserId.
        cell: ({ row }) => memberNameByUserId[row.original.createdByUserId] ?? row.original.createdByUserId,
      },
      {
        accessorKey: "createdAt",
        header: t("history.date"),
        cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
      },
    ],
    [memberNameByUserId, t],
  );

  return (
    <DataTable
      columns={columns}
      data={transactions}
      searchPlaceholder={t("history.search")}
      exportFilename="referral-earnings"
      emptyTitle={t("history.emptyTitle")}
      emptyDescription={t("history.emptyDescription")}
      pageSize={10}
    />
  );
}

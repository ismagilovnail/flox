"use client";

import type { ColumnDef } from "@tanstack/react-table";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { RelativeTime } from "@/components/ui/relative-time";
import { Mono } from "@/components/ui/typography";
import { usePushBroadcasts } from "@/hooks/use-push";
import { formatInt, formatPercent } from "@/lib/format";
import type { PushBroadcast } from "@/lib/api/push";

function num(n: number) {
  return <Mono className="tabular-nums">{formatInt(n)}</Mono>;
}

function broadcastColumns(t: TFunction): ColumnDef<typeof dataTableFeatures, PushBroadcast>[] {
  return [
    {
      accessorKey: "name",
      header: t("broadcasts.columns.name"),
      cell: ({ row }) => (
        <div className="flex min-w-48 flex-col">
          <span className="font-medium">{row.original.name}</span>
          <span className="truncate text-xs text-muted-foreground">{row.original.title}</span>
        </div>
      ),
    },
    {
      accessorKey: "createdAt",
      header: t("broadcasts.columns.sent"),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} className="text-xs text-muted-foreground" />,
    },
    { accessorKey: "total", header: t("broadcasts.columns.audience"), cell: ({ getValue }) => num(getValue() as number) },
    { accessorKey: "delivered", header: t("broadcasts.columns.delivered"), cell: ({ getValue }) => num(getValue() as number) },
    {
      accessorKey: "failed",
      header: t("broadcasts.columns.failed"),
      cell: ({ row }) => (
        <div className="flex flex-col">
          {num(row.original.failed)}
          {row.original.unsubscribed > 0 && (
            <span className="text-xs text-muted-foreground">{t("broadcasts.failedHint", { count: row.original.unsubscribed })}</span>
          )}
        </div>
      ),
    },
    {
      id: "pending",
      accessorFn: (b) => Math.max(0, b.total - b.delivered - b.failed),
      header: t("broadcasts.columns.pending"),
      cell: ({ getValue }) => num(getValue() as number),
    },
    { accessorKey: "opened", header: t("broadcasts.columns.opened"), cell: ({ getValue }) => num(getValue() as number) },
    {
      id: "ctr",
      // Opens over delivered; nothing delivered yet is "—", not 0%.
      accessorFn: (b) => (b.delivered > 0 ? (b.opened / b.delivered) * 100 : null),
      header: t("broadcasts.columns.ctr"),
      cell: ({ getValue }) => {
        const v = getValue() as number | null;
        return <Mono className="tabular-nums">{v === null ? "—" : formatPercent(v)}</Mono>;
      },
    },
  ];
}

export function BroadcastList() {
  const { t } = useTranslation("push");
  const query = usePushBroadcasts();

  if (query.isPending) return <LoadingState label={t("broadcasts.loading")} />;
  if (query.isError) {
    return <ErrorState title={t("broadcasts.loadError")} description={query.error.message} onRetry={() => query.refetch()} />;
  }
  return (
    <DataTable
      columns={broadcastColumns(t)}
      data={query.data.broadcasts}
      searchPlaceholder={t("broadcasts.searchPlaceholder")}
      exportFilename="push-broadcasts"
      emptyTitle={t("broadcasts.emptyTitle")}
      emptyDescription={t("broadcasts.emptyDescription")}
      pageSize={20}
    />
  );
}

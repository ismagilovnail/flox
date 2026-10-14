"use client";

import * as React from "react";
import type { ColumnDef } from "@tanstack/react-table";
import type { TFunction } from "i18next";
import { CalendarXIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { RelativeTime } from "@/components/ui/relative-time";
import { Mono } from "@/components/ui/typography";
import { usePermission } from "@/hooks/use-auth";
import { useOrganization } from "@/hooks/use-organization";
import { useCancelPushBroadcast, usePushBroadcasts } from "@/hooks/use-push";
import { formatInt, formatPercent } from "@/lib/format";
import { formatInZone } from "@/lib/timezones";
import type { PushBroadcast } from "@/lib/api/push";

function num(n: number) {
  return <Mono className="tabular-nums">{formatInt(n)}</Mono>;
}

function broadcastColumns(
  t: TFunction,
  opts: { timeZone: string; locale: string; onCancel?: (b: PushBroadcast) => void },
): ColumnDef<typeof dataTableFeatures, PushBroadcast>[] {
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
      id: "when",
      accessorFn: (b) => b.sentAt ?? b.sendAt ?? b.createdAt,
      header: t("broadcasts.columns.sent"),
      cell: ({ row }) => {
        const b = row.original;
        if (b.status === "scheduled" && b.sendAt) {
          return (
            <div className="flex flex-col items-start gap-0.5">
              <Badge variant="warning">{t("broadcasts.status.scheduled")}</Badge>
              <span className="text-xs text-muted-foreground">{formatInZone(new Date(b.sendAt), opts.timeZone, opts.locale)}</span>
            </div>
          );
        }
        if (b.status === "canceled") return <Badge variant="secondary">{t("broadcasts.status.canceled")}</Badge>;
        return <RelativeTime date={b.sentAt ?? b.createdAt} className="text-xs text-muted-foreground" />;
      },
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
    ...(opts.onCancel
      ? [
          {
            id: "actions",
            header: "",
            enableSorting: false,
            cell: ({ row }) =>
              row.original.status === "scheduled" ? (
                <IconButton aria-label={t("broadcasts.cancelAria", { name: row.original.name })} size="icon-sm" onClick={() => opts.onCancel?.(row.original)}>
                  <CalendarXIcon className="size-3.5" />
                </IconButton>
              ) : null,
          } satisfies ColumnDef<typeof dataTableFeatures, PushBroadcast>,
        ]
      : []),
  ];
}

export function BroadcastList() {
  const { t, i18n } = useTranslation("push");
  const query = usePushBroadcasts();
  const canWrite = usePermission("push.write");
  const timeZone = useOrganization().data?.timezone || "UTC";
  const cancel = useCancelPushBroadcast();
  const [canceling, setCanceling] = React.useState<PushBroadcast | null>(null);

  function confirmCancel() {
    if (!canceling) return;
    cancel.mutate(canceling.id, {
      onSuccess: (b) => toast(t("broadcasts.canceled"), { description: b.name }),
      onError: (err) => toast.error(t("broadcasts.cancelError"), { description: err.message }),
      onSettled: () => setCanceling(null),
    });
  }

  if (query.isPending) return <LoadingState label={t("broadcasts.loading")} />;
  if (query.isError) {
    return <ErrorState title={t("broadcasts.loadError")} description={query.error.message} onRetry={() => query.refetch()} />;
  }
  return (
    <>
      <DataTable
        columns={broadcastColumns(t, { timeZone, locale: i18n.language, onCancel: canWrite ? setCanceling : undefined })}
        data={query.data.broadcasts}
        searchPlaceholder={t("broadcasts.searchPlaceholder")}
        exportFilename="push-broadcasts"
        emptyTitle={t("broadcasts.emptyTitle")}
        emptyDescription={t("broadcasts.emptyDescription")}
        pageSize={20}
      />
      <Dialog open={canceling !== null} onOpenChange={(o) => !o && !cancel.isPending && setCanceling(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("broadcasts.cancelTitle", { name: canceling?.name ?? "" })}</DialogTitle>
            <DialogDescription>{t("broadcasts.cancelDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCanceling(null)} disabled={cancel.isPending}>
              {t("broadcasts.keep")}
            </Button>
            <Button variant="destructive" onClick={confirmCancel} disabled={cancel.isPending}>
              {t("broadcasts.cancelConfirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

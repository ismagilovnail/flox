import type { ColumnDef } from "@tanstack/react-table";
import { ArrowDownToLineIcon, ArrowUpFromLineIcon, RotateCcwIcon } from "lucide-react";
import { formatDistanceToNow } from "date-fns";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { IconButton } from "@/components/ui/icon-button";
import { Caption, Mono } from "@/components/ui/typography";
import { CPA_STATUS_I18N_KEY } from "@/lib/api/conversions";
import { useReplayIncomingPostback, useReplayOutgoingPostback } from "@/hooks/use-postback-logs";
import type { PostbackLog, PostbackResult } from "@/lib/api/postback-logs";

const RESULT_VARIANT: Record<PostbackResult, "success" | "warning" | "danger" | "secondary"> = {
  success: "success",
  duplicate: "warning",
  ignored: "secondary",
  error: "danger",
  queued: "secondary",
  processing: "secondary",
  retrying: "warning",
  failed: "danger",
  dead: "danger",
};

/** Outgoing: re-enqueues a fresh delivery through the same path a first
 * attempt already takes (§46, apps/internal/postback.Store.Enqueue).
 * Incoming: re-runs the attempt through the conversion engine, the same
 * call apps/tracker's own /postback/{networkId} makes for a real network
 * hit (§45) — dedup/status-progression rules apply exactly as they would
 * for a genuine retry, so the toast surfaces whatever result actually
 * came back (success/duplicate/ignored/error), not a fixed message. */
function PostbackReplayButton({ log }: { log: PostbackLog }) {
  const { t } = useTranslation("postbacks");
  const replayOutgoing = useReplayOutgoingPostback();
  const replayIncoming = useReplayIncomingPostback();

  if (log.direction === "outgoing") {
    if (!log.status || !log.url) return null;
    return (
      <IconButton
        aria-label={t("logs.replayAria", { clickId: log.clickId })}
        size="icon-sm"
        disabled={replayOutgoing.isPending}
        onClick={() =>
          replayOutgoing.mutate(
            { networkId: log.networkId, clickId: log.clickId, status: log.status!, eventRef: log.eventRef, url: log.url! },
            {
              onSuccess: () => toast(t("logs.toast.replayed")),
              onError: (err) => toast.error(t("logs.toast.replayError"), { description: err.message }),
            },
          )
        }
      >
        <RotateCcwIcon className="size-3.5" />
      </IconButton>
    );
  }

  if (log.direction === "incoming" && log.rawStatus) {
    return (
      <IconButton
        aria-label={t("logs.replayIncomingAria", { clickId: log.clickId })}
        size="icon-sm"
        disabled={replayIncoming.isPending}
        onClick={() =>
          replayIncoming.mutate(
            {
              networkId: log.networkId,
              clickId: log.clickId,
              rawStatus: log.rawStatus!,
              eventRef: log.eventRef,
              revenue: log.revenue,
              currency: log.currency,
            },
            {
              onSuccess: (result) =>
                toast(t("logs.toast.replayedIncoming"), { description: t(`result.${result.result}`, { ns: "postbacks" }) }),
              onError: (err) => toast.error(t("logs.toast.replayIncomingError"), { description: err.message }),
            },
          )
        }
      >
        <RotateCcwIcon className="size-3.5" />
      </IconButton>
    );
  }

  return null;
}

export function postbackLogColumns(
  t: TFunction,
  networkNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, PostbackLog>[] {
  return [
    {
      accessorKey: "direction",
      header: t("columns.direction", { ns: "postbacks" }),
      cell: ({ getValue }) => {
        const direction = getValue() as PostbackLog["direction"];
        const Icon = direction === "incoming" ? ArrowDownToLineIcon : ArrowUpFromLineIcon;
        return (
          <span className="flex items-center gap-1.5 text-xs">
            <Icon className="size-3.5 text-muted-foreground" /> {t(`direction.${direction}`, { ns: "postbacks" })}
          </span>
        );
      },
    },
    {
      id: "network",
      header: t("columns.network", { ns: "postbacks" }),
      accessorFn: (row) => networkNameById[row.networkId] ?? row.networkId,
    },
    {
      accessorKey: "clickId",
      header: t("columns.clickId", { ns: "postbacks" }),
      cell: ({ getValue }) => <Mono className="text-xs">{getValue() as string}</Mono>,
    },
    {
      id: "status",
      header: t("columns.status", { ns: "postbacks" }),
      cell: ({ row }) => {
        const { rawStatus, status } = row.original;
        if (rawStatus) {
          return (
            <span className="flex items-center gap-1 text-xs">
              <Mono>{rawStatus}</Mono>
              {status && (
                <span className="text-muted-foreground">→ {t(CPA_STATUS_I18N_KEY[status], { ns: "conversions" })}</span>
              )}
            </span>
          );
        }
        if (status) return <Mono className="text-xs">{t(CPA_STATUS_I18N_KEY[status], { ns: "conversions" })}</Mono>;
        return <span className="text-xs text-muted-foreground">—</span>;
      },
    },
    {
      accessorKey: "result",
      header: t("columns.result", { ns: "postbacks" }),
      cell: ({ getValue }) => {
        const result = getValue() as PostbackResult;
        return <Badge variant={RESULT_VARIANT[result]}>{t(`result.${result}`, { ns: "postbacks" })}</Badge>;
      },
    },
    {
      accessorKey: "message",
      header: t("columns.message", { ns: "postbacks" }),
      cell: ({ getValue }) => <Caption className="block max-w-xs truncate">{getValue() as string}</Caption>,
    },
    {
      accessorKey: "eventAt",
      header: t("columns.time", { ns: "postbacks" }),
      cell: ({ getValue }) => <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>,
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <PostbackReplayButton log={row.original} />
        </div>
      ),
    },
  ];
}

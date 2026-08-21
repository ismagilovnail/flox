import type { ColumnDef } from "@tanstack/react-table";
import { ArrowDownToLineIcon, ArrowUpFromLineIcon } from "lucide-react";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
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

/** No replay action: it's a real write path (re-invoking the conversion
 * engine for an incoming row, or re-enqueuing a delivery for an outgoing
 * one) deliberately scoped out of this phase — see docs/postback-logs.md. */
export function postbackLogColumns(networkNameById: Record<string, string>): ColumnDef<typeof dataTableFeatures, PostbackLog>[] {
  return [
    {
      accessorKey: "direction",
      header: "Direction",
      cell: ({ getValue }) => {
        const direction = getValue() as PostbackLog["direction"];
        const Icon = direction === "incoming" ? ArrowDownToLineIcon : ArrowUpFromLineIcon;
        return (
          <span className="flex items-center gap-1.5 text-xs capitalize">
            <Icon className="size-3.5 text-muted-foreground" /> {direction}
          </span>
        );
      },
    },
    {
      id: "network",
      header: "Network",
      accessorFn: (row) => networkNameById[row.networkId] ?? row.networkId,
    },
    {
      accessorKey: "clickId",
      header: "Click ID",
      cell: ({ getValue }) => <Mono className="text-xs">{getValue() as string}</Mono>,
    },
    {
      id: "status",
      header: "Status",
      cell: ({ row }) => {
        const { rawStatus, status } = row.original;
        if (rawStatus) {
          return (
            <span className="flex items-center gap-1 text-xs">
              <Mono>{rawStatus}</Mono>
              {status && <span className="text-muted-foreground">→ {status.replace("CPA_", "")}</span>}
            </span>
          );
        }
        if (status) return <Mono className="text-xs">{status.replace("CPA_", "")}</Mono>;
        return <span className="text-xs text-muted-foreground">—</span>;
      },
    },
    {
      accessorKey: "result",
      header: "Result",
      cell: ({ getValue }) => {
        const result = getValue() as PostbackResult;
        return <Badge variant={RESULT_VARIANT[result]}>{result}</Badge>;
      },
    },
    {
      accessorKey: "message",
      header: "Message",
      cell: ({ getValue }) => <Caption className="block max-w-xs truncate">{getValue() as string}</Caption>,
    },
    {
      accessorKey: "eventAt",
      header: "Time",
      cell: ({ getValue }) => <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>,
    },
  ];
}

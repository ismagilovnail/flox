import type { ColumnDef } from "@tanstack/react-table";
import { RotateCcwIcon, ArrowDownToLineIcon, ArrowUpFromLineIcon } from "lucide-react";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { IconButton } from "@/components/ui/icon-button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Caption, Mono } from "@/components/ui/typography";
import type { PostbackLog, PostbackResult } from "@/lib/mock/postback-logs";

const RESULT_VARIANT: Record<PostbackResult, "success" | "warning" | "danger"> = {
  success: "success",
  duplicate: "warning",
  error: "danger",
};

export function postbackLogColumns(
  networkNameById: Record<string, string>,
  onReplay: (log: PostbackLog) => void,
): ColumnDef<typeof dataTableFeatures, PostbackLog>[] {
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
        const { rawStatus, mappedStatus } = row.original;
        if (!rawStatus) return <span className="text-xs text-muted-foreground">—</span>;
        return (
          <span className="flex items-center gap-1 text-xs">
            <Mono>{rawStatus}</Mono>
            {mappedStatus && <span className="text-muted-foreground">→ {mappedStatus.replace("CPA_", "")}</span>}
          </span>
        );
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
      accessorKey: "createdAt",
      header: "Time",
      cell: ({ getValue }) => (
        <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>
      ),
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton
                aria-label={`Replay postback for ${row.original.clickId}`}
                size="icon-sm"
                onClick={() => onReplay(row.original)}
              >
                <RotateCcwIcon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>Replay</TooltipContent>
          </Tooltip>
        </div>
      ),
    },
  ];
}

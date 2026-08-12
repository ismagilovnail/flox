import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import type { Postlanding, PostlandingStatus } from "@/lib/mock/postlandings";
import { PostlandingRowActions } from "@/features/postlanding/postlanding-row-actions";

const STATUS_VARIANT: Record<PostlandingStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function postlandingColumns(
  onEdit: (postlanding: Postlanding) => void,
): ColumnDef<typeof dataTableFeatures, Postlanding>[] {
  return [
    {
      accessorKey: "name",
      header: "Name",
      cell: ({ row }) => (
        <button
          type="button"
          onClick={() => onEdit(row.original)}
          className="font-medium text-foreground hover:underline"
        >
          {row.original.name}
        </button>
      ),
    },
    {
      accessorKey: "url",
      header: "URL",
      cell: ({ getValue }) => <Mono className="block max-w-md truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "events",
      header: "Events",
      cell: ({ getValue }) => {
        const events = getValue() as string[];
        return (
          <div className="flex flex-wrap gap-1">
            {events.slice(0, 2).map((e) => (
              <Badge key={e} variant="outline">
                {e}
              </Badge>
            ))}
            {events.length > 2 && <Badge variant="outline">+{events.length - 2}</Badge>}
          </div>
        );
      },
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as PostlandingStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
    },
    {
      accessorKey: "updatedAt",
      header: "Updated",
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
          <PostlandingRowActions postlanding={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

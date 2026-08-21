import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import type { Network, NetworkStatus } from "@/lib/api/networks";
import { NetworkRowActions } from "@/features/networks/network-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<NetworkStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function networkColumns(onEdit: (network: Network) => void): ColumnDef<typeof dataTableFeatures, Network>[] {
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
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as NetworkStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
    },
    {
      id: "tags",
      header: "Tags",
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="network" entityId={row.original.id} />,
    },
    {
      accessorKey: "postbackUrl",
      header: "Postback URL",
      cell: ({ getValue }) => <Mono className="block max-w-md truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "acceptDuplicates",
      header: "Dedup",
      cell: ({ getValue }) => (
        <Badge variant={getValue() ? "warning" : "outline"}>{getValue() ? "accepts dupes" : "dedup on"}</Badge>
      ),
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
          <NetworkRowActions network={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

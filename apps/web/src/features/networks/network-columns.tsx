import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import type { TFunction } from "i18next";

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

export function networkColumns(t: TFunction, onEdit: (network: Network) => void): ColumnDef<typeof dataTableFeatures, Network>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "networks" }),
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
      header: t("columns.status", { ns: "networks" }),
      cell: ({ getValue }) => {
        const status = getValue() as NetworkStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      id: "tags",
      header: t("columns.tags", { ns: "networks" }),
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="network" entityId={row.original.id} />,
    },
    {
      accessorKey: "postbackUrl",
      header: t("columns.postbackUrl", { ns: "networks" }),
      cell: ({ getValue }) => <Mono className="block max-w-md truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "acceptDuplicates",
      header: t("columns.dedup", { ns: "networks" }),
      cell: ({ getValue }) => (
        <Badge variant={getValue() ? "warning" : "outline"}>
          {getValue() ? t("dedup.acceptsDuplicates", { ns: "networks" }) : t("dedup.dedupOn", { ns: "networks" })}
        </Badge>
      ),
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "networks" }),
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

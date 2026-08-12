import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { COST_INTEGRATION_LABELS, type SourceStatus, type TrafficSource } from "@/lib/mock/traffic-sources";
import { SourceRowActions } from "@/features/traffic-sources/source-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<SourceStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function sourceColumns(onEdit: (source: TrafficSource) => void): ColumnDef<typeof dataTableFeatures, TrafficSource>[] {
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
    { accessorKey: "type", header: "Type" },
    {
      accessorKey: "trackingTemplate",
      header: "Tracking template",
      cell: ({ getValue }) => <Mono className="block max-w-md truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "costIntegration",
      header: "Cost integration",
      cell: ({ getValue }) => {
        const integration = getValue() as TrafficSource["costIntegration"];
        return (
          <Badge variant={integration === "none" ? "outline" : "secondary"}>
            {COST_INTEGRATION_LABELS[integration]}
          </Badge>
        );
      },
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as SourceStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
    },
    {
      id: "tags",
      header: "Tags",
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="traffic_source" entityId={row.original.id} />,
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
          <SourceRowActions source={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

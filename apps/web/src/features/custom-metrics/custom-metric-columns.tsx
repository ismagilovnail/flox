import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { SHOW_IN_TARGETS } from "@/lib/mock/custom-metrics-registry";
import type { CustomMetric, CustomMetricStatus } from "@/lib/mock/custom-metrics";
import { CustomMetricRowActions } from "@/features/custom-metrics/custom-metric-row-actions";

const STATUS_VARIANT: Record<CustomMetricStatus, "success" | "outline"> = {
  published: "success",
  draft: "outline",
};

export function customMetricColumns(
  onEdit: (metric: CustomMetric) => void,
  canManage: (metric: CustomMetric) => boolean,
): ColumnDef<typeof dataTableFeatures, CustomMetric>[] {
  return [
    {
      accessorKey: "name",
      header: "Name",
      cell: ({ row }) =>
        canManage(row.original) ? (
          <button
            type="button"
            onClick={() => onEdit(row.original)}
            className="font-medium text-foreground hover:underline"
          >
            {row.original.name}
          </button>
        ) : (
          <span className="font-medium text-foreground">{row.original.name}</span>
        ),
    },
    { accessorKey: "group", header: "Group" },
    {
      accessorKey: "formula",
      header: "Formula",
      cell: ({ getValue }) => <Mono className="block max-w-xs truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as CustomMetricStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
    },
    {
      accessorKey: "active",
      header: "Active",
      cell: ({ getValue }) => <Badge variant={getValue() ? "success" : "secondary"}>{getValue() ? "on" : "off"}</Badge>,
    },
    {
      id: "targets",
      header: "Show in",
      cell: ({ row }) => (
        <div className="flex flex-wrap gap-1">
          {row.original.targets.length === 0 ? (
            <Caption className="text-muted-foreground">Not exposed</Caption>
          ) : (
            row.original.targets.map((t) => (
              <Badge key={t} variant="outline">
                {SHOW_IN_TARGETS.find((s) => s.id === t)?.label ?? t}
              </Badge>
            ))
          )}
        </div>
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
      cell: ({ row }) =>
        canManage(row.original) ? (
          <div className="flex justify-end">
            <CustomMetricRowActions metric={row.original} onEdit={() => onEdit(row.original)} />
          </div>
        ) : null,
    },
  ];
}

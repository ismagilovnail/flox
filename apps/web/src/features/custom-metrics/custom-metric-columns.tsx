import type { ColumnDef } from "@tanstack/react-table";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { SHOW_IN_TARGETS } from "@/lib/custom-metrics-registry";
import type { CustomMetric, CustomMetricStatus } from "@/lib/custom-metrics";
import { CustomMetricRowActions } from "@/features/custom-metrics/custom-metric-row-actions";
import { targetLabel } from "@/features/custom-metrics/metric-label";

const STATUS_VARIANT: Record<CustomMetricStatus, "success" | "outline"> = {
  published: "success",
  draft: "outline",
};

export function customMetricColumns(
  onEdit: (metric: CustomMetric) => void,
  canManage: (metric: CustomMetric) => boolean,
  t: TFunction<"customMetrics">,
): ColumnDef<typeof dataTableFeatures, CustomMetric>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name"),
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
    { accessorKey: "group", header: t("columns.group") },
    {
      accessorKey: "formula",
      header: t("columns.formula"),
      cell: ({ getValue }) => <Mono className="block max-w-xs truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "status",
      header: t("columns.status"),
      cell: ({ getValue }) => {
        const status = getValue() as CustomMetricStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`statuses.${status}`)}</Badge>;
      },
    },
    {
      accessorKey: "active",
      header: t("columns.active"),
      cell: ({ getValue }) => (
        <Badge variant={getValue() ? "success" : "secondary"}>{getValue() ? t("active.on") : t("active.off")}</Badge>
      ),
    },
    {
      id: "targets",
      header: t("columns.showIn"),
      cell: ({ row }) => (
        <div className="flex flex-wrap gap-1">
          {row.original.targets.length === 0 ? (
            <Caption className="text-muted-foreground">{t("notShown")}</Caption>
          ) : (
            row.original.targets.map((target) => (
              <Badge key={target} variant="outline">
                {SHOW_IN_TARGETS.some((s) => s.id === target) ? targetLabel(t, target) : target}
              </Badge>
            ))
          )}
        </div>
      ),
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated"),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
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

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { COST_INTEGRATION_I18N_KEY, SOURCE_TYPE_I18N_KEY, type SourceStatus, type SourceType, type TrafficSource } from "@/lib/api/traffic-sources";
import { SourceRowActions } from "@/features/traffic-sources/source-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<SourceStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function sourceColumns(t: TFunction): ColumnDef<typeof dataTableFeatures, TrafficSource>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "trafficSources" }),
      cell: ({ row }) => (
        <Link href={`/traffic-sources/${row.original.id}`} className="font-medium text-foreground hover:underline">
          {row.original.name}
        </Link>
      ),
    },
    {
      accessorKey: "type",
      header: t("columns.type", { ns: "trafficSources" }),
      cell: ({ getValue }) => {
        const type = getValue() as string;
        const key = SOURCE_TYPE_I18N_KEY[type as SourceType];
        return key ? t(key, { ns: "trafficSources" }) : type;
      },
    },
    {
      accessorKey: "trackingTemplate",
      header: t("columns.trackingTemplate", { ns: "trafficSources" }),
      cell: ({ getValue }) => <Mono className="block max-w-md truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "costIntegration",
      header: t("columns.costIntegration", { ns: "trafficSources" }),
      cell: ({ getValue }) => {
        const integration = getValue() as TrafficSource["costIntegration"];
        return (
          <Badge variant={integration === "none" ? "outline" : "secondary"}>
            {t(COST_INTEGRATION_I18N_KEY[integration], { ns: "trafficSources" })}
          </Badge>
        );
      },
    },
    {
      accessorKey: "status",
      header: t("columns.status", { ns: "trafficSources" }),
      cell: ({ getValue }) => {
        const status = getValue() as SourceStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      id: "tags",
      header: t("columns.tags", { ns: "trafficSources" }),
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="traffic_source" entityId={row.original.id} />,
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "trafficSources" }),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <SourceRowActions source={row.original} />
        </div>
      ),
    },
  ];
}

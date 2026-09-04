import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import type { Landing, LandingStatus } from "@/lib/api/landings";
import { LandingRowActions } from "@/features/landings/landing-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<LandingStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function landingColumns(t: TFunction): ColumnDef<typeof dataTableFeatures, Landing>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "landings" }),
      cell: ({ row }) => (
        <Link href={`/landings/${row.original.id}`} className="font-medium text-foreground hover:underline">
          {row.original.name}
        </Link>
      ),
    },
    {
      accessorKey: "type",
      header: t("columns.type", { ns: "landings" }),
      cell: ({ getValue }) => {
        const type = getValue() as Landing["type"];
        return <Badge variant="outline">{t(`type.${type}`, { ns: "landings" })}</Badge>;
      },
    },
    {
      accessorKey: "url",
      header: t("columns.url", { ns: "landings" }),
      cell: ({ getValue }) => <Mono className="block max-w-md truncate text-xs">{getValue() as string}</Mono>,
    },
    {
      accessorKey: "status",
      header: t("columns.status", { ns: "landings" }),
      cell: ({ getValue }) => {
        const status = getValue() as LandingStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      id: "tags",
      header: t("columns.tags", { ns: "landings" }),
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="landing" entityId={row.original.id} />,
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "landings" }),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <LandingRowActions landing={row.original} />
        </div>
      ),
    },
  ];
}

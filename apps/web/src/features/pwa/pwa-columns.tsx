import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import type { Pwa, PwaStatus } from "@/lib/api/pwa";
import { PwaRowActions } from "@/features/pwa/pwa-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<PwaStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function pwaColumns(t: TFunction, onEdit: (pwa: Pwa) => void): ColumnDef<typeof dataTableFeatures, Pwa>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "pwa" }),
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
    { accessorKey: "shortName", header: t("columns.shortName", { ns: "pwa" }) },
    {
      id: "theme",
      header: t("columns.theme", { ns: "pwa" }),
      cell: ({ row }) => (
        <div className="flex items-center gap-1.5">
          <span
            className="size-3.5 rounded-full ring-1 ring-border"
            style={{ backgroundColor: row.original.themeColor }}
          />
          <Mono className="text-xs">{row.original.themeColor}</Mono>
        </div>
      ),
    },
    {
      accessorKey: "bounceInAppWebview",
      header: t("columns.webviewBounce", { ns: "pwa" }),
      cell: ({ getValue }) => (
        <Badge variant={getValue() ? "success" : "outline"}>
          {getValue() ? t("webviewBounce.on", { ns: "pwa" }) : t("webviewBounce.off", { ns: "pwa" })}
        </Badge>
      ),
    },
    {
      accessorKey: "status",
      header: t("columns.status", { ns: "pwa" }),
      cell: ({ getValue }) => {
        const status = getValue() as PwaStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      id: "tags",
      header: t("columns.tags", { ns: "pwa" }),
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="pwa" entityId={row.original.id} />,
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "pwa" }),
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
          <PwaRowActions pwa={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

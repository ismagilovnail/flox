import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { LayoutTemplateIcon } from "lucide-react";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import type { Landing, LandingStatus } from "@/lib/api/landings";
import { LandingRowActions } from "@/features/landings/landing-row-actions";
import { formatBytes } from "@/features/landings/format-bytes";
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
        <div className="flex min-w-0 items-center gap-2.5">
          {row.original.logoUrl ? (
            // eslint-disable-next-line @next/next/no-img-element -- a Content Gallery public object-storage URL, not a Next-optimizable static asset
            <img src={row.original.logoUrl} alt="" className="size-8 shrink-0 rounded-md border object-cover" />
          ) : (
            <div className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted text-muted-foreground">
              <LayoutTemplateIcon className="size-4" />
            </div>
          )}
          <div className="grid min-w-0">
            <Link href={`/landings/${row.original.id}`} className="truncate font-medium text-foreground hover:underline">
              {row.original.name}
            </Link>
            {row.original.title && <span className="truncate text-xs text-muted-foreground">{row.original.title}</span>}
          </div>
        </div>
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
      // Where the landing lives: the advertiser's URL for external, the
      // uploaded bundle's shape for internal (its asset base URL isn't a
      // page anyone should open directly).
      id: "source",
      accessorFn: (l) =>
        l.type === "external" ? l.url : `${l.entryFile} · ${l.fileCount} · ${formatBytes(l.bundleSize)}`,
      header: t("columns.source", { ns: "landings" }),
      cell: ({ row }) =>
        row.original.type === "external" ? (
          <Mono className="block max-w-md truncate text-xs">{row.original.url}</Mono>
        ) : (
          <span className="text-xs text-muted-foreground">
            <Mono>{row.original.entryFile}</Mono> ·{" "}
            <span className="tabular-nums">
              {t("bundle.fileCount", { ns: "landings", count: row.original.fileCount })} · {formatBytes(row.original.bundleSize)}
            </span>
          </span>
        ),
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

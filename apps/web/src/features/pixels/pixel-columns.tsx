import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { PIXEL_PROVIDER_I18N_KEY, type Pixel, type PixelStatus } from "@/lib/api/pixels";
import { PixelRowActions } from "@/features/pixels/pixel-row-actions";

const STATUS_VARIANT: Record<PixelStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function pixelColumns(t: TFunction, onEdit: (pixel: Pixel) => void): ColumnDef<typeof dataTableFeatures, Pixel>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "pixels" }),
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
      accessorKey: "provider",
      header: t("columns.provider", { ns: "pixels" }),
      cell: ({ getValue }) => <Badge variant="outline">{t(PIXEL_PROVIDER_I18N_KEY[getValue() as Pixel["provider"]], { ns: "pixels" })}</Badge>,
    },
    {
      accessorKey: "pixelId",
      header: t("columns.pixelId", { ns: "pixels" }),
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string) || "—"}</Mono>,
    },
    {
      accessorKey: "events",
      header: t("columns.events", { ns: "pixels" }),
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
      header: t("columns.status", { ns: "pixels" }),
      cell: ({ getValue }) => {
        const status = getValue() as PixelStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "pixels" }),
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
          <PixelRowActions pixel={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

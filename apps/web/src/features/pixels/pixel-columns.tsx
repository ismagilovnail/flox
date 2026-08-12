import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { PIXEL_PROVIDER_LABELS, type Pixel, type PixelStatus } from "@/lib/mock/pixels";
import { PixelRowActions } from "@/features/pixels/pixel-row-actions";

const STATUS_VARIANT: Record<PixelStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function pixelColumns(onEdit: (pixel: Pixel) => void): ColumnDef<typeof dataTableFeatures, Pixel>[] {
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
      accessorKey: "provider",
      header: "Provider",
      cell: ({ getValue }) => <Badge variant="outline">{PIXEL_PROVIDER_LABELS[getValue() as Pixel["provider"]]}</Badge>,
    },
    {
      accessorKey: "pixelId",
      header: "Pixel ID",
      cell: ({ getValue }) => <Mono className="text-xs">{(getValue() as string) || "—"}</Mono>,
    },
    {
      accessorKey: "events",
      header: "Events",
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
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as PixelStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
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
          <PixelRowActions pixel={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

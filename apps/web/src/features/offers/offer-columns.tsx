import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import type { Offer, OfferStatus } from "@/lib/mock/offers";
import { OfferRowActions } from "@/features/offers/offer-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<OfferStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function offerColumns(
  onEdit: (offer: Offer) => void,
  networkNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, Offer>[] {
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
      id: "network",
      header: "Network",
      accessorFn: (row) => networkNameById[row.networkId] ?? row.networkId,
    },
    {
      accessorKey: "countries",
      header: "GEOs",
      cell: ({ getValue }) => {
        const countries = getValue() as string[];
        return (
          <div className="flex flex-wrap gap-1">
            {countries.slice(0, 3).map((c) => (
              <Badge key={c} variant="outline">
                {c}
              </Badge>
            ))}
            {countries.length > 3 && <Badge variant="outline">+{countries.length - 3}</Badge>}
          </div>
        );
      },
    },
    {
      id: "payout",
      header: "Payout",
      accessorFn: (row) => row.payout,
      cell: ({ row }) => <Mono>{row.original.payout.toFixed(2)} {row.original.currency}</Mono>,
    },
    {
      accessorKey: "cap",
      header: "Daily Cap",
      cell: ({ getValue }) => {
        const cap = getValue() as number | null;
        return <Mono>{cap === null ? "Uncapped" : cap.toLocaleString("en-US")}</Mono>;
      },
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as OfferStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status}</Badge>;
      },
    },
    {
      id: "tags",
      header: "Tags",
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="offer" entityId={row.original.id} />,
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
          <OfferRowActions offer={row.original} onEdit={() => onEdit(row.original)} />
        </div>
      ),
    },
  ];
}

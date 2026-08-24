import type { ColumnDef } from "@tanstack/react-table";
import { formatDistanceToNow } from "date-fns";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import type { Offer, OfferStatus } from "@/lib/api/offers";
import { OfferRowActions } from "@/features/offers/offer-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<OfferStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

export function offerColumns(
  t: TFunction,
  onEdit: (offer: Offer) => void,
  networkNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, Offer>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "offers" }),
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
      header: t("columns.network", { ns: "offers" }),
      accessorFn: (row) => networkNameById[row.networkId] ?? row.networkId,
    },
    {
      accessorKey: "countries",
      header: t("columns.geos", { ns: "offers" }),
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
      header: t("columns.payout", { ns: "offers" }),
      accessorFn: (row) => row.payout,
      cell: ({ row }) => (
        <Mono>
          {row.original.payout.toFixed(2)} {row.original.currency}
        </Mono>
      ),
    },
    {
      accessorKey: "cap",
      header: t("columns.dailyCap", { ns: "offers" }),
      cell: ({ getValue }) => {
        const cap = getValue() as number | null;
        return <Mono>{cap === null ? t("columns.uncapped", { ns: "offers" }) : cap.toLocaleString("en-US")}</Mono>;
      },
    },
    {
      accessorKey: "status",
      header: t("columns.status", { ns: "offers" }),
      cell: ({ getValue }) => {
        const status = getValue() as OfferStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      id: "tags",
      header: t("columns.tags", { ns: "offers" }),
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="offer" entityId={row.original.id} />,
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "offers" }),
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

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
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
  networkNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, Offer>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "offers" }),
      cell: ({ row }) => (
        <Link href={`/offers/${row.original.id}`} className="font-medium text-foreground hover:underline">
          {row.original.name}
        </Link>
      ),
    },
    {
      id: "network",
      header: t("columns.network", { ns: "offers" }),
      // A `cell` renderer, not `accessorFn` — see campaign-columns.tsx's own
      // comment: accessorFn's return value is cached per row and only
      // re-evaluated when the `data` array's reference changes, not when
      // networkNameById (a separate, later-resolving query) changes. An
      // accessorFn here would risk freezing on the raw networkId fallback
      // if this column is ever read before networks finish loading.
      cell: ({ row }) => networkNameById[row.original.networkId] ?? row.original.networkId,
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
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <OfferRowActions offer={row.original} />
        </div>
      ),
    },
  ];
}

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import type { Conversion, CpaStatus, PostbackDeliveryStatus } from "@/lib/mock/conversions";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

const POSTBACK_VARIANT: Record<PostbackDeliveryStatus, "success" | "warning" | "danger" | "outline"> = {
  sent: "success",
  pending: "warning",
  failed: "danger",
  not_configured: "outline",
};

export function conversionColumns(
  campaignNameById: Record<string, string>,
  offerNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, Conversion>[] {
  return [
    {
      accessorKey: "clickId",
      header: "Click ID",
      cell: ({ row }) => (
        <Link href={`/conversions/${row.original.id}`} className="font-mono text-xs text-foreground hover:underline">
          {row.original.clickId}
        </Link>
      ),
    },
    {
      id: "campaign",
      header: "Campaign",
      accessorFn: (row) => campaignNameById[row.campaignId] ?? row.campaignId,
    },
    {
      id: "offer",
      header: "Offer",
      accessorFn: (row) => offerNameById[row.offerId] ?? row.offerId,
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = getValue() as CpaStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{status.replace("CPA_", "")}</Badge>;
      },
    },
    {
      accessorKey: "revenue",
      header: "Revenue",
      cell: ({ row }) => (
        <Mono>
          {row.original.revenue === 0 ? "—" : `${row.original.revenue.toFixed(2)} ${row.original.currency}`}
        </Mono>
      ),
    },
    {
      accessorKey: "postbackStatus",
      header: "Postback",
      cell: ({ getValue }) => {
        const status = getValue() as PostbackDeliveryStatus;
        return <Badge variant={POSTBACK_VARIANT[status]}>{status.replace("_", " ")}</Badge>;
      },
    },
    {
      accessorKey: "eventAt",
      header: "Event time",
      cell: ({ getValue }) => (
        <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>
      ),
    },
  ];
}

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { formatDistanceToNow } from "date-fns";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import type { Conversion, CpaStatus } from "@/lib/api/conversions";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

/** No Offer column: conversion_events carries no offer_id (only flow_id,
 * which would need a separate Postgres join to resolve — out of scope,
 * see lib/api/conversions.ts). No Postback column: outgoing delivery
 * status is the deferred Postback Logs domain, not faked here. */
export function conversionColumns(
  campaignNameById: Record<string, string>,
  networkNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, Conversion>[] {
  return [
    {
      accessorKey: "clickId",
      header: "Click ID",
      cell: ({ row }) => (
        <Link href={`/conversions/${row.original.clickId}`} className="font-mono text-xs text-foreground hover:underline">
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
      id: "network",
      header: "Network",
      accessorFn: (row) => networkNameById[row.networkId] ?? row.networkId,
    },
    {
      accessorKey: "type",
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
        <Mono>{row.original.revenue === 0 ? "—" : `${row.original.revenue.toFixed(2)} ${row.original.currency}`}</Mono>
      ),
    },
    {
      accessorKey: "eventAt",
      header: "Event time",
      cell: ({ getValue }) => <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>,
    },
  ];
}

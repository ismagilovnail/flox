import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { formatDistanceToNow } from "date-fns";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption, Mono } from "@/components/ui/typography";
import { CPA_STATUS_I18N_KEY, type Conversion, type CpaStatus } from "@/lib/api/conversions";

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
  t: TFunction,
  campaignNameById: Record<string, string>,
  networkNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, Conversion>[] {
  return [
    {
      accessorKey: "clickId",
      header: t("columns.clickId", { ns: "conversions" }),
      cell: ({ row }) => (
        <Link href={`/conversions/${row.original.clickId}`} className="font-mono text-xs text-foreground hover:underline">
          {row.original.clickId}
        </Link>
      ),
    },
    {
      id: "campaign",
      header: t("columns.campaign", { ns: "conversions" }),
      accessorFn: (row) => campaignNameById[row.campaignId] ?? row.campaignId,
    },
    {
      id: "network",
      header: t("columns.network", { ns: "conversions" }),
      accessorFn: (row) => networkNameById[row.networkId] ?? row.networkId,
    },
    {
      accessorKey: "type",
      header: t("columns.status", { ns: "conversions" }),
      cell: ({ getValue }) => {
        const status = getValue() as CpaStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(CPA_STATUS_I18N_KEY[status], { ns: "conversions" })}</Badge>;
      },
    },
    {
      accessorKey: "revenue",
      header: t("columns.revenue", { ns: "conversions" }),
      cell: ({ row }) => (
        <Mono>{row.original.revenue === 0 ? "—" : `${row.original.revenue.toFixed(2)} ${row.original.currency}`}</Mono>
      ),
    },
    {
      accessorKey: "eventAt",
      header: t("columns.eventTime", { ns: "conversions" }),
      cell: ({ getValue }) => <Caption>{formatDistanceToNow(new Date(getValue() as string), { addSuffix: true })}</Caption>,
    },
  ];
}

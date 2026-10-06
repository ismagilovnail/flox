import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { format } from "date-fns";
import { CONVERSION_TAG_KEYS, CPA_STATUS_I18N_KEY, type Conversion, type CpaStatus } from "@/lib/api/conversions";

const STATUS_VARIANT: Record<CpaStatus, "warning" | "success" | "danger" | "secondary"> = {
  CPA_HOLD: "warning",
  CPA_ACCEPT: "success",
  CPA_REDEP: "success",
  CPA_DECLINE: "danger",
  CPA_TRASH: "secondary",
};

/** Columns hidden until the buyer turns them on in "Columns" — the reconciliation
 * detail (USD, network txn id, utm_*, sub1–sub10). */
export const CONVERSION_HIDDEN_BY_DEFAULT: Record<string, boolean> = Object.fromEntries(
  ["usdValue", "networkTxnId", ...CONVERSION_TAG_KEYS].map((id) => [id, false]),
);

const TAG_HEADER: Record<(typeof CONVERSION_TAG_KEYS)[number], string> = {
  utmSource: "utm_source",
  utmMedium: "utm_medium",
  utmCampaign: "utm_campaign",
  utmContent: "utm_content",
  utmTerm: "utm_term",
  sub1: "sub1",
  sub2: "sub2",
  sub3: "sub3",
  sub4: "sub4",
  sub5: "sub5",
  sub6: "sub6",
  sub7: "sub7",
  sub8: "sub8",
  sub9: "sub9",
  sub10: "sub10",
};

/** No Postback column: outgoing delivery status is the deferred Postback Logs
 * domain, not faked here. */
export function conversionColumns(
  t: TFunction,
  campaignNameById: Record<string, string>,
  networkNameById: Record<string, string>,
  flowNameById: Record<string, string>,
  offerNameById: Record<string, string>,
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
      // `cell`, not `accessorFn` — see campaign-columns.tsx's own comment:
      // accessorFn's return value is cached per row and only re-evaluated
      // when the `data` array's reference changes, not when campaignNameById
      // (a separate, later-resolving query) changes.
      cell: ({ row }) => campaignNameById[row.original.campaignId] ?? row.original.campaignId,
    },
    {
      id: "network",
      header: t("columns.network", { ns: "conversions" }),
      cell: ({ row }) => networkNameById[row.original.networkId] ?? row.original.networkId,
    },
    {
      id: "offer",
      header: t("columns.offer", { ns: "conversions" }),
      cell: ({ row }) => {
        const id = row.original.offerId;
        return id ? (offerNameById[id] ?? id) : "—";
      },
    },
    {
      id: "flow",
      header: t("columns.flow", { ns: "conversions" }),
      cell: ({ row }) => {
        const id = row.original.flowId;
        return id ? (flowNameById[id] ?? id) : "—";
      },
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
      id: "history",
      header: t("columns.history", { ns: "conversions" }),
      enableSorting: false,
      // Every status this click_id reached, oldest first: hold → accept → redep.
      cell: ({ row }) => (
        <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
          {row.original.history.map((step, i) => (
            <span key={`${step.type}:${step.eventAt}`} className="inline-flex items-center gap-1.5">
              {i > 0 && <span className="text-muted-foreground">→</span>}
              <Badge variant={STATUS_VARIANT[step.type]}>{t(CPA_STATUS_I18N_KEY[step.type], { ns: "conversions" })}</Badge>
              <span className="font-mono text-xs tabular-nums text-muted-foreground">
                {format(new Date(step.eventAt), "dd.MM HH:mm")}
              </span>
            </span>
          ))}
        </div>
      ),
    },
    {
      accessorKey: "revenue",
      header: t("columns.revenue", { ns: "conversions" }),
      cell: ({ row }) => <Mono>{row.original.revenue === 0 ? "—" : row.original.revenue.toFixed(2)}</Mono>,
    },
    {
      accessorKey: "currency",
      header: t("columns.currency", { ns: "conversions" }),
      cell: ({ row }) => <Mono>{row.original.revenue === 0 ? "—" : row.original.currency || "—"}</Mono>,
    },
    {
      accessorKey: "usdValue",
      header: t("columns.usdValue", { ns: "conversions" }),
      // "No FX rate on file" is not zero (CLAUDE.md #7): it shows as "—".
      cell: ({ row }) => <Mono>{row.original.hasUsdValue ? row.original.usdValue.toFixed(2) : "—"}</Mono>,
    },
    {
      accessorKey: "networkTxnId",
      header: t("columns.networkTxnId", { ns: "conversions" }),
      cell: ({ row }) => (row.original.networkTxnId ? <Mono className="text-xs">{row.original.networkTxnId}</Mono> : "—"),
    },
    {
      accessorKey: "eventAt",
      header: t("columns.eventTime", { ns: "conversions" }),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
    },
    ...CONVERSION_TAG_KEYS.map((key) => ({
      id: key,
      accessorKey: key,
      header: TAG_HEADER[key],
      cell: ({ row }: { row: { original: Conversion } }) =>
        row.original[key] ? <Mono className="text-xs">{row.original[key]}</Mono> : "—",
    })),
  ];
}

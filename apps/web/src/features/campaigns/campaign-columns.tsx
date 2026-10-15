import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import type { CampaignStatus } from "@/lib/api/campaigns";
import type { CustomMetric } from "@/lib/custom-metrics";
import { evaluateFormula } from "@/lib/formula-engine";
import { METRICS, formatMetric, metricLabel } from "@/lib/analytics-registry";
import { CAMPAIGN_TABLE_METRICS, type CampaignRow } from "@/features/campaigns/campaign-metrics";
import { CampaignRowActions } from "@/features/campaigns/campaign-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<CampaignStatus, "success" | "warning" | "outline" | "secondary"> = {
  active: "success",
  paused: "warning",
  draft: "outline",
  archived: "secondary",
};

/** A metric cell: a skeleton while the period's report loads, nothing when
 * it failed (the list shows the error above the table), else the value —
 * "—" for a null, which is how a missing cost reads (CLAUDE.md #6). */
function metricCell(row: CampaignRow, loading: boolean, value: (row: CampaignRow) => number | null, format: (v: number | null) => string) {
  if (!row.metrics) return loading ? <Skeleton className="ml-auto h-4 w-12" /> : null;
  return <Mono className="block text-right">{format(value(row))}</Mono>;
}

/** sourceNameById resolves each row's trafficSourceId to a display name —
 * the real API returns only the id. The metric columns read the period's
 * numbers each row carries (withCampaignMetrics); customMetrics are the
 * ones published to the campaigns table, evaluated per row by the same
 * formula engine as the report builder, on the table's metrics only. */
export function campaignColumns(
  t: TFunction,
  sourceNameById: Record<string, string>,
  customMetrics: CustomMetric[] = [],
  metricsLoading = false,
  currency = "USD",
): ColumnDef<typeof dataTableFeatures, CampaignRow>[] {
  const metricColumns: ColumnDef<typeof dataTableFeatures, CampaignRow>[] = [
    ...CAMPAIGN_TABLE_METRICS.map((key): ColumnDef<typeof dataTableFeatures, CampaignRow> => {
      const format = METRICS.find((m) => m.key === key)!.format;
      return {
        id: key,
        header: key === "cost" ? t("columns.spend", { ns: "campaigns" }) : metricLabel(t, key),
        accessorFn: (row) => row.metrics?.[key] ?? null,
        sortUndefined: "last",
        cell: ({ row }) => metricCell(row.original, metricsLoading, (r) => r.metrics?.[key] ?? null, (v) => formatMetric(v, format, currency)),
      };
    }),
    ...customMetrics.map((cm): ColumnDef<typeof dataTableFeatures, CampaignRow> => ({
      id: `cm_${cm.id}`,
      header: cm.name,
      accessorFn: (row) => (row.metrics ? evaluateFormula(cm.formula, row.metrics) : null),
      cell: ({ row }) =>
        metricCell(
          row.original,
          metricsLoading,
          (r) => (r.metrics ? evaluateFormula(cm.formula, r.metrics) : null),
          (v) => formatMetric(v, cm.format, currency),
        ),
    })),
  ];

  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "campaigns" }),
      cell: ({ row }) => (
        <Link href={`/campaigns/${row.original.id}`} className="font-medium text-foreground hover:underline">
          {row.original.name}
        </Link>
      ),
    },
    {
      accessorKey: "status",
      header: t("columns.status", { ns: "campaigns" }),
      cell: ({ getValue }) => {
        const status = getValue() as CampaignStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      id: "source",
      header: t("columns.source", { ns: "campaigns" }),
      // A `cell` renderer, not `accessorFn` — TanStack Table caches a row's
      // computed value per (row, columnId) the first time it's read, and
      // only invalidates it when the `data` array's own reference changes,
      // not when an accessorFn's closed-over external state (sourceNameById,
      // populated asynchronously by a separate query) changes. An accessorFn
      // here would risk freezing on the raw trafficSourceId fallback if this
      // column is ever read before sources finish loading. `cell` runs fresh
      // on every render, so it always reflects the current sourceNameById.
      cell: ({ row }) => sourceNameById[row.original.trafficSourceId] ?? row.original.trafficSourceId,
    },
    ...metricColumns,
    {
      id: "tags",
      header: t("columns.tags", { ns: "campaigns" }),
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="campaign" entityId={row.original.id} />,
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "campaigns" }),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <CampaignRowActions campaign={row.original} />
        </div>
      ),
    },
  ];
}

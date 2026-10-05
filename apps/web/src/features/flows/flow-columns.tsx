import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { BarChart3Icon } from "lucide-react";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { IconButton } from "@/components/ui/icon-button";
import { Mono } from "@/components/ui/typography";
import { RelativeTime } from "@/components/ui/relative-time";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { FlowSummary } from "@/lib/api/flows";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";

/**
 * weightSumByStreamSetId: weight is a raw integer (Flow.Weight), only
 * meaningful normalized against the sum of every OTHER flow in the same
 * stream set (§24 pickWeighted) — the exact same computation
 * stream-set-row.tsx's own weightSum already does, just precomputed
 * once across the whole cross-campaign list here (flow-list.tsx) rather
 * than locally per stream set. offerNameById resolves a destination's
 * offerId the same client-side way stream-set-row.tsx's own
 * destinationLabel does — no backend join, matching the rest of this
 * codebase's "resolve names from an already-fetched list" convention.
 */
export function flowColumns(
  t: TFunction,
  weightSumByStreamSetId: Record<string, number>,
  offerNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, FlowSummary>[] {
  return [
    {
      accessorKey: "name",
      header: t("columns.name", { ns: "flows" }),
      cell: ({ row }) => <span className="font-medium text-foreground">{row.original.name}</span>,
    },
    {
      id: "campaign",
      header: t("columns.campaign", { ns: "flows" }),
      accessorFn: (row) => row.campaignName,
      cell: ({ row }) => (
        <Link href={`/campaigns/${row.original.campaignId}`} className="hover:underline">
          {row.original.campaignName}
        </Link>
      ),
    },
    {
      id: "streamSet",
      header: t("columns.streamSet", { ns: "flows" }),
      accessorFn: (row) => row.streamSetName,
    },
    {
      id: "weight",
      header: t("columns.weight", { ns: "flows" }),
      cell: ({ row }) => {
        const sum = weightSumByStreamSetId[row.original.streamSetId] ?? 0;
        const pct = sum > 0 ? Math.round((row.original.weight / sum) * 100) : 0;
        return <Mono>{pct}%</Mono>;
      },
    },
    {
      id: "status",
      header: t("columns.status", { ns: "flows" }),
      accessorFn: (row) => (row.active ? "active" : "paused"),
      cell: ({ row }) => (
        <Badge variant={row.original.active ? "success" : "secondary"}>
          {t(`status.${row.original.active ? "active" : "paused"}`, { ns: "common" })}
        </Badge>
      ),
    },
    {
      id: "destination",
      header: t("columns.destination", { ns: "flows" }),
      cell: ({ row }) => {
        const d = row.original.destination;
        if (d.kind === "redirect") {
          return <span className="text-muted-foreground">{t("destination.redirect", { ns: "flows" })}</span>;
        }
        return offerNameById[d.offerId] ?? d.offerId;
      },
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "flows" }),
      cell: ({ getValue }) => <RelativeTime date={getValue() as string} />,
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("row.viewStatisticsAria", { ns: "flows" })} size="icon-sm" asChild>
                <Link href={viewStatisticsHref("flow", row.original.id)}>
                  <BarChart3Icon className="size-3.5" />
                </Link>
              </IconButton>
            </TooltipTrigger>
            <TooltipContent className="max-w-64 text-center">{t("row.viewStatisticsTooltip", { ns: "flows" })}</TooltipContent>
          </Tooltip>
        </div>
      ),
    },
  ];
}

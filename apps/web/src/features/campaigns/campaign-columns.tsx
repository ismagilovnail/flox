import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { formatDistanceToNow } from "date-fns";
import type { TFunction } from "i18next";

import { dataTableFeatures } from "@/components/ui/data-table";
import { Badge } from "@/components/ui/badge";
import { Caption } from "@/components/ui/typography";
import type { Campaign, CampaignStatus } from "@/lib/api/campaigns";
import { CampaignRowActions } from "@/features/campaigns/campaign-row-actions";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

const STATUS_VARIANT: Record<CampaignStatus, "success" | "warning" | "outline" | "secondary"> = {
  active: "success",
  paused: "warning",
  draft: "outline",
  archived: "secondary",
};

/** Volume/revenue columns (clicks/conversions/revenue/spend/profit/ROI)
 * that the Phase 13-era mock table showed are gone here — the real
 * Campaign model carries none of them, and a per-row bulk analytics
 * rollup across every campaign in the list doesn't exist yet (only a
 * single-campaign query, apps/internal/analytics). See
 * docs/frontend-integration.md. sourceNameById resolves each row's
 * trafficSourceId to a display name — the real API returns only the id. */
export function campaignColumns(
  t: TFunction,
  sourceNameById: Record<string, string>,
): ColumnDef<typeof dataTableFeatures, Campaign>[] {
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
      accessorFn: (row) => sourceNameById[row.trafficSourceId] ?? row.trafficSourceId,
    },
    {
      id: "tags",
      header: t("columns.tags", { ns: "campaigns" }),
      enableSorting: false,
      cell: ({ row }) => <TagBadgeList entityType="campaign" entityId={row.original.id} />,
    },
    {
      accessorKey: "updatedAt",
      header: t("columns.updated", { ns: "campaigns" }),
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
          <CampaignRowActions campaign={row.original} />
        </div>
      ),
    },
  ];
}

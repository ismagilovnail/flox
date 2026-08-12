"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";

import { Button } from "@/components/ui/button";
import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { Mono } from "@/components/ui/typography";
import { useCampaignsStore } from "@/stores/campaigns";
import { useTagsStore } from "@/stores/tags";
import { useCustomMetricsStore } from "@/stores/custom-metrics";
import { campaignColumns } from "@/features/campaigns/campaign-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import { evaluateFormula } from "@/lib/formula-engine";
import { formatMetric } from "@/features/analytics/registry";
import type { Campaign } from "@/lib/mock/campaigns";

/** Maps a Campaign row onto the same registry metric ids the formula engine
 * uses everywhere else — only the fields a campaign row actually carries. */
function campaignMetricValues(c: Campaign): Record<string, number | null> {
  const profit = c.spend === null ? null : c.revenue - c.spend;
  const roi = c.spend && c.spend > 0 ? ((profit as number) / c.spend) * 100 : null;
  return { clicks: c.clicks, conversions: c.conversions, revenue: c.revenue, cost: c.spend, profit, roi };
}

export function CampaignList() {
  const campaigns = useCampaignsStore((s) => s.campaigns);
  const assignments = useTagsStore((s) => s.assignments);
  const allCustomMetrics = useCustomMetricsStore((s) => s.metrics);

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const filtered = React.useMemo(
    () => filterByTags("campaign", campaigns, tagFilter, assignments),
    [campaigns, tagFilter, assignments],
  );

  const columns = React.useMemo(() => {
    const customColumns: ColumnDef<typeof dataTableFeatures, Campaign>[] = allCustomMetrics
      .filter((m) => m.status === "published" && m.active && m.targets.includes("campaigns_table"))
      .map((cm) => ({
        id: `cm_${cm.id}`,
        header: cm.name,
        accessorFn: (row: Campaign) => evaluateFormula(cm.formula, campaignMetricValues(row)),
        cell: ({ getValue }: { getValue: () => unknown }) => (
          <Mono>{formatMetric(getValue() as number | null, cm.format)}</Mono>
        ),
      }));
    // Insert before the trailing "actions" column so it stays rightmost.
    return [...campaignColumns.slice(0, -1), ...customColumns, campaignColumns[campaignColumns.length - 1]];
  }, [allCustomMetrics]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Campaigns</h1>
        <Button asChild>
          <Link href="/campaigns/new">
            <PlusIcon className="size-4" />
            New Campaign
          </Link>
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder="Search campaigns..."
        emptyTitle="No campaigns yet"
        emptyDescription="Create your first campaign to start routing traffic."
        pageSize={10}
        filters={<TagFilterControl selected={tagFilter} onChange={setTagFilter} />}
        enableRowSelection
        getRowId={(row) => row.id}
        bulkActions={({ selectedRows, clearSelection }) => (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setBulkTarget({ ids: selectedRows.map((r) => r.id), clear: clearSelection })}
          >
            <TagIcon className="size-3.5" /> Edit Tags
          </Button>
        )}
      />

      {bulkTarget && (
        <BulkTagDialog
          entityType="campaign"
          entityIds={bulkTarget.ids}
          open
          onOpenChange={(open) => !open && setBulkTarget(null)}
          onApplied={() => {
            bulkTarget.clear();
            setBulkTarget(null);
          }}
        />
      )}
    </div>
  );
}

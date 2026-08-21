"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { useTagsStore } from "@/stores/tags";
import { campaignColumns } from "@/features/campaigns/campaign-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function CampaignList() {
  const campaignsQuery = useCampaigns();
  const sourcesQuery = useTrafficSources();
  const assignments = useTagsStore((s) => s.assignments);

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const sourceNameById = React.useMemo(() => {
    const map: Record<string, string> = {};
    for (const s of sourcesQuery.data?.trafficSources ?? []) map[s.id] = s.name;
    return map;
  }, [sourcesQuery.data]);

  const filtered = React.useMemo(
    () => filterByTags("campaign", campaignsQuery.data?.campaigns ?? [], tagFilter, assignments),
    [campaignsQuery.data, tagFilter, assignments],
  );

  const columns = React.useMemo(() => campaignColumns(sourceNameById), [sourceNameById]);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">Campaigns</h1>
      <Button asChild>
        <Link href="/campaigns/new">
          <PlusIcon className="size-4" />
          New Campaign
        </Link>
      </Button>
    </div>
  );

  if (campaignsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label="Loading campaigns…" />
      </div>
    );
  }

  if (campaignsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title="Couldn't load campaigns"
          description={campaignsQuery.error.message}
          onRetry={() => campaignsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {header}

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

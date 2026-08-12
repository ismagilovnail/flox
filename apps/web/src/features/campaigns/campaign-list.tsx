"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useCampaignsStore } from "@/stores/campaigns";
import { useTagsStore } from "@/stores/tags";
import { campaignColumns } from "@/features/campaigns/campaign-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function CampaignList() {
  const campaigns = useCampaignsStore((s) => s.campaigns);
  const assignments = useTagsStore((s) => s.assignments);

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const filtered = React.useMemo(
    () => filterByTags("campaign", campaigns, tagFilter, assignments),
    [campaigns, tagFilter, assignments],
  );

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
        columns={campaignColumns}
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

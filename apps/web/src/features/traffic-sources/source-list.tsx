"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useTrafficSourcesStore } from "@/stores/traffic-sources";
import { useTagsStore } from "@/stores/tags";
import { sourceColumns } from "@/features/traffic-sources/source-columns";
import { SourceFormSheet, type SourceFormValues } from "@/features/traffic-sources/source-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { TrafficSource } from "@/lib/mock/traffic-sources";

export function SourceList() {
  const sources = useTrafficSourcesStore((s) => s.sources);
  const addSource = useTrafficSourcesStore((s) => s.addSource);
  const updateSource = useTrafficSourcesStore((s) => s.updateSource);
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<TrafficSource | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  function handleSubmit(values: SourceFormValues) {
    if (target) {
      updateSource(target.id, values);
      toast("Source updated", { description: values.name });
    } else {
      addSource(values);
      toast("Source created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => sourceColumns((source) => setTarget(source)), []);
  const filtered = React.useMemo(
    () => filterByTags("traffic_source", sources, tagFilter, assignments),
    [sources, tagFilter, assignments],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Traffic Sources</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Source
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder="Search sources..."
        emptyTitle="No traffic sources yet"
        emptyDescription="Add the places your traffic comes from — campaigns reference these by name."
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

      {target !== undefined && (
        <SourceFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Source"}
          submitLabel={target ? "Save changes" : "Create source"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}

      {bulkTarget && (
        <BulkTagDialog
          entityType="traffic_source"
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

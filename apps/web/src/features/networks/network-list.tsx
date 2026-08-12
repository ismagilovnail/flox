"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useNetworksStore } from "@/stores/networks";
import { useTagsStore } from "@/stores/tags";
import { networkColumns } from "@/features/networks/network-columns";
import { NetworkFormSheet, type NetworkFormValues } from "@/features/networks/network-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Network } from "@/lib/mock/networks";

export function NetworkList() {
  const networks = useNetworksStore((s) => s.networks);
  const addNetwork = useNetworksStore((s) => s.addNetwork);
  const updateNetwork = useNetworksStore((s) => s.updateNetwork);
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<Network | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  function handleSubmit(values: NetworkFormValues) {
    if (target) {
      updateNetwork(target.id, values);
      toast("Network updated", { description: values.name });
    } else {
      addNetwork(values);
      toast("Network created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => networkColumns((network) => setTarget(network)), []);
  const filtered = React.useMemo(
    () => filterByTags("network", networks, tagFilter, assignments),
    [networks, tagFilter, assignments],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Networks</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Network
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder="Search networks..."
        emptyTitle="No networks yet"
        emptyDescription="Add the CPA/CPL networks your offers belong to."
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
        <NetworkFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Network"}
          submitLabel={target ? "Save changes" : "Create network"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
        />
      )}

      {bulkTarget && (
        <BulkTagDialog
          entityType="network"
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

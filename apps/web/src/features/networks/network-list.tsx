"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCreateNetwork, useNetworks, useUpdateNetwork } from "@/hooks/use-networks";
import { useTagsStore } from "@/stores/tags";
import { networkColumns } from "@/features/networks/network-columns";
import { NetworkFormSheet, type NetworkFormValues } from "@/features/networks/network-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Network } from "@/lib/api/networks";

export function NetworkList() {
  const networksQuery = useNetworks();
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<Network | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => networkColumns((network) => setTarget(network)), []);
  const filtered = React.useMemo(
    () => filterByTags("network", networksQuery.data?.networks ?? [], tagFilter, assignments),
    [networksQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">Networks</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        New Network
      </Button>
    </div>
  );

  if (networksQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label="Loading networks…" />
      </div>
    );
  }

  if (networksQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title="Couldn't load networks"
          description={networksQuery.error.message}
          onRetry={() => networksQuery.refetch()}
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
        <NetworkFormDialog key={target?.id ?? "new"} target={target} onClose={() => setTarget(undefined)} />
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

function NetworkFormDialog({ target, onClose }: { target: Network | null; onClose: () => void }) {
  const createNetwork = useCreateNetwork();
  const updateNetwork = useUpdateNetwork(target?.id ?? "");

  function handleSubmit(values: NetworkFormValues) {
    if (target) {
      updateNetwork.mutate(
        { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates, status: values.status },
        {
          onSuccess: () => {
            toast("Network updated", { description: values.name });
            onClose();
          },
          onError: (err) => toast.error("Couldn't update network", { description: err.message }),
        },
      );
    } else {
      createNetwork.mutate(
        { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates },
        {
          onSuccess: () => {
            toast("Network created", { description: values.name });
            onClose();
          },
          onError: (err) => toast.error("Couldn't create network", { description: err.message }),
        },
      );
    }
  }

  return (
    <NetworkFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={target ? `Edit ${target.name}` : "New Network"}
      submitLabel={target ? "Save changes" : "Create network"}
      showStatus={!!target}
      defaultValues={
        target
          ? { name: target.name, postbackUrl: target.postbackUrl, acceptDuplicates: target.acceptDuplicates, status: target.status }
          : {}
      }
      onSubmit={handleSubmit}
    />
  );
}

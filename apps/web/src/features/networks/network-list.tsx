"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation(["networks", "common"]);
  const networksQuery = useNetworks();
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<Network | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => networkColumns(t, (network) => setTarget(network)), [t]);
  const filtered = React.useMemo(
    () => filterByTags("network", networksQuery.data?.networks ?? [], tagFilter, assignments),
    [networksQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "networks" })}</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        {t("list.newButton", { ns: "networks" })}
      </Button>
    </div>
  );

  if (networksQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "networks" })} />
      </div>
    );
  }

  if (networksQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "networks" })}
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
        searchPlaceholder={t("list.searchPlaceholder", { ns: "networks" })}
        emptyTitle={t("list.emptyTitle", { ns: "networks" })}
        emptyDescription={t("list.emptyDescription", { ns: "networks" })}
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
            <TagIcon className="size-3.5" /> {t("list.editTagsButton", { ns: "networks" })}
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
  const { t } = useTranslation("networks");
  const createNetwork = useCreateNetwork();
  const updateNetwork = useUpdateNetwork(target?.id ?? "");

  function handleSubmit(values: NetworkFormValues) {
    if (target) {
      updateNetwork.mutate(
        { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates, status: values.status },
        {
          onSuccess: () => {
            toast(t("toast.updated"), { description: values.name });
            onClose();
          },
          onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
        },
      );
    } else {
      createNetwork.mutate(
        { name: values.name, postbackUrl: values.postbackUrl, acceptDuplicates: values.acceptDuplicates },
        {
          onSuccess: () => {
            toast(t("toast.created"), { description: values.name });
            onClose();
          },
          onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
        },
      );
    }
  }

  return (
    <NetworkFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={target ? t("form.titleEdit", { name: target.name }) : t("form.titleNew")}
      submitLabel={target ? t("form.submitEdit") : t("form.submitCreate")}
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

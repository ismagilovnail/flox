"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCreateTrafficSource, useTrafficSources, useUpdateTrafficSource } from "@/hooks/use-traffic-sources";
import { useTagsStore } from "@/stores/tags";
import { sourceColumns } from "@/features/traffic-sources/source-columns";
import { SourceFormSheet, type SourceFormValues } from "@/features/traffic-sources/source-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { TrafficSource } from "@/lib/api/traffic-sources";

export function SourceList() {
  const { t } = useTranslation(["trafficSources", "common"]);
  const sourcesQuery = useTrafficSources();
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<TrafficSource | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => sourceColumns(t, (source) => setTarget(source)), [t]);
  const filtered = React.useMemo(
    () => filterByTags("traffic_source", sourcesQuery.data?.trafficSources ?? [], tagFilter, assignments),
    [sourcesQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "trafficSources" })}</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        {t("list.newButton", { ns: "trafficSources" })}
      </Button>
    </div>
  );

  if (sourcesQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "trafficSources" })} />
      </div>
    );
  }

  if (sourcesQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "trafficSources" })}
          description={sourcesQuery.error.message}
          onRetry={() => sourcesQuery.refetch()}
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
        searchPlaceholder={t("list.searchPlaceholder", { ns: "trafficSources" })}
        emptyTitle={t("list.emptyTitle", { ns: "trafficSources" })}
        emptyDescription={t("list.emptyDescription", { ns: "trafficSources" })}
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
            <TagIcon className="size-3.5" /> {t("list.editTagsButton", { ns: "trafficSources" })}
          </Button>
        )}
      />

      {target !== undefined && (
        <SourceFormDialog key={target?.id ?? "new"} target={target} onClose={() => setTarget(undefined)} />
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

/** Owns its own create/update mutation, scoped to whichever source (or
 * "new") is being edited — a fresh instance per target (via `key` in the
 * parent) rather than one shared mutation hook, since
 * useUpdateTrafficSource(id) needs a real id and "new" has none yet. */
function SourceFormDialog({ target, onClose }: { target: TrafficSource | null; onClose: () => void }) {
  const { t } = useTranslation("trafficSources");
  const createSource = useCreateTrafficSource();
  const updateSource = useUpdateTrafficSource(target?.id ?? "");

  function handleSubmit(values: SourceFormValues) {
    if (target) {
      updateSource.mutate(
        {
          name: values.name,
          type: values.type,
          trackingTemplate: values.trackingTemplate,
          costIntegration: values.costIntegration,
          status: values.status,
        },
        {
          onSuccess: () => {
            toast(t("toast.updated"), { description: values.name });
            onClose();
          },
          onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
        },
      );
    } else {
      createSource.mutate(
        {
          name: values.name,
          type: values.type,
          trackingTemplate: values.trackingTemplate,
          costIntegration: values.costIntegration,
        },
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
    <SourceFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={target ? t("form.titleEdit", { name: target.name }) : t("form.titleNew")}
      submitLabel={target ? t("form.submitEdit") : t("form.submitCreate")}
      showStatus={!!target}
      defaultValues={
        target
          ? {
              name: target.name,
              type: target.type as SourceFormValues["type"],
              trackingTemplate: target.trackingTemplate,
              costIntegration: target.costIntegration,
              status: target.status,
            }
          : {}
      }
      onSubmit={handleSubmit}
    />
  );
}

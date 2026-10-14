"use client";

import * as React from "react";
import { TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useFlows } from "@/hooks/use-flows";
import { useOffers } from "@/hooks/use-offers";
import { EMPTY_TAG_ASSIGNMENTS, useTagAssignments } from "@/hooks/use-tags";
import { flowColumns } from "@/features/flows/flow-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function FlowList() {
  const { t } = useTranslation("flows");
  const flowsQuery = useFlows();
  const offersQuery = useOffers();
  const assignments = useTagAssignments("flow").data?.assignments ?? EMPTY_TAG_ASSIGNMENTS;
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);
  const filtered = React.useMemo(
    () => filterByTags("flow", flowsQuery.data?.flows ?? [], tagFilter, assignments),
    [flowsQuery.data, tagFilter, assignments],
  );

  const offerNameById = React.useMemo(
    () => Object.fromEntries((offersQuery.data?.offers ?? []).map((o) => [o.id, o.name])),
    [offersQuery.data],
  );

  const weightSumByStreamSetId = React.useMemo(() => {
    const sums: Record<string, number> = {};
    for (const f of flowsQuery.data?.flows ?? []) {
      sums[f.streamSetId] = (sums[f.streamSetId] ?? 0) + f.weight;
    }
    return sums;
  }, [flowsQuery.data]);

  const columns = React.useMemo(
    () => flowColumns(t, weightSumByStreamSetId, offerNameById),
    [t, weightSumByStreamSetId, offerNameById],
  );

  if (flowsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>
        <LoadingState label={t("list.loading")} />
      </div>
    );
  }

  if (flowsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>
        <ErrorState
          title={t("list.loadError")}
          description={flowsQuery.error.message}
          onRetry={() => flowsQuery.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("list.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("list.description")}</p>
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        getRowId={(row) => row.id}
        exportFilename="flows"
        searchPlaceholder={t("list.searchPlaceholder")}
        emptyTitle={t("list.emptyTitle")}
        emptyDescription={t("list.emptyDescription")}
        pageSize={20}
        enableColumnReorder
        persistKey="flows"
        filters={<TagFilterControl selected={tagFilter} onChange={setTagFilter} />}
        enableRowSelection
        bulkActions={({ selectedRows, clearSelection }) => (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setBulkTarget({ ids: selectedRows.map((r) => r.id), clear: clearSelection })}
          >
            <TagIcon className="size-3.5" /> {t("list.editTagsButton")}
          </Button>
        )}
      />

      {bulkTarget && (
        <BulkTagDialog
          entityType="flow"
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

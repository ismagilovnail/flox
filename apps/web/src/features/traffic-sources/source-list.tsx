"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { EMPTY_TAG_ASSIGNMENTS, useTagAssignments } from "@/hooks/use-tags";
import { sourceColumns } from "@/features/traffic-sources/source-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function SourceList() {
  const { t } = useTranslation(["trafficSources", "common"]);
  const sourcesQuery = useTrafficSources();
  const assignments = useTagAssignments("traffic_source").data?.assignments ?? EMPTY_TAG_ASSIGNMENTS;

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => sourceColumns(t), [t]);
  const filtered = React.useMemo(
    () => filterByTags("traffic_source", sourcesQuery.data?.trafficSources ?? [], tagFilter, assignments),
    [sourcesQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "trafficSources" })}</h1>
      <Button asChild>
        <Link href="/traffic-sources/new">
          <PlusIcon className="size-4" />
          {t("list.newButton", { ns: "trafficSources" })}
        </Link>
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

"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useLandings } from "@/hooks/use-landings";
import { useTagsStore } from "@/stores/tags";
import { landingColumns } from "@/features/landings/landing-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function LandingList() {
  const { t } = useTranslation(["landings", "common"]);
  const landingsQuery = useLandings();
  const assignments = useTagsStore((s) => s.assignments);

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => landingColumns(t), [t]);
  const filtered = React.useMemo(
    () => filterByTags("landing", landingsQuery.data?.landings ?? [], tagFilter, assignments),
    [landingsQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "landings" })}</h1>
      <Button asChild>
        <Link href="/landings/new">
          <PlusIcon className="size-4" />
          {t("list.newButton", { ns: "landings" })}
        </Link>
      </Button>
    </div>
  );

  if (landingsQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "landings" })} />
      </div>
    );
  }

  if (landingsQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "landings" })}
          description={landingsQuery.error.message}
          onRetry={() => landingsQuery.refetch()}
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
        searchPlaceholder={t("list.searchPlaceholder", { ns: "landings" })}
        emptyTitle={t("list.emptyTitle", { ns: "landings" })}
        emptyDescription={t("list.emptyDescription", { ns: "landings" })}
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
            <TagIcon className="size-3.5" /> {t("list.editTagsButton", { ns: "landings" })}
          </Button>
        )}
      />

      {bulkTarget && (
        <BulkTagDialog
          entityType="landing"
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

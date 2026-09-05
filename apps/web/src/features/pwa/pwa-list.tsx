"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePwas } from "@/hooks/use-pwas";
import { EMPTY_TAG_ASSIGNMENTS, useTagAssignments } from "@/hooks/use-tags";
import { pwaColumns } from "@/features/pwa/pwa-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function PwaList() {
  const { t } = useTranslation(["pwa", "common"]);
  const pwasQuery = usePwas();
  const assignments = useTagAssignments("pwa").data?.assignments ?? EMPTY_TAG_ASSIGNMENTS;

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => pwaColumns(t), [t]);
  const filtered = React.useMemo(
    () => filterByTags("pwa", pwasQuery.data?.pwas ?? [], tagFilter, assignments),
    [pwasQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "pwa" })}</h1>
      <Button asChild>
        <Link href="/pwa/new">
          <PlusIcon className="size-4" />
          {t("list.newButton", { ns: "pwa" })}
        </Link>
      </Button>
    </div>
  );

  if (pwasQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "pwa" })} />
      </div>
    );
  }

  if (pwasQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "pwa" })}
          description={pwasQuery.error.message}
          onRetry={() => pwasQuery.refetch()}
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
        searchPlaceholder={t("list.searchPlaceholder", { ns: "pwa" })}
        emptyTitle={t("list.emptyTitle", { ns: "pwa" })}
        emptyDescription={t("list.emptyDescription", { ns: "pwa" })}
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
            <TagIcon className="size-3.5" /> {t("list.editTagsButton", { ns: "pwa" })}
          </Button>
        )}
      />

      {bulkTarget && (
        <BulkTagDialog
          entityType="pwa"
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

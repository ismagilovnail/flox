"use client";

import * as React from "react";
import Link from "next/link";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useOffers } from "@/hooks/use-offers";
import { useNetworks } from "@/hooks/use-networks";
import { useTagsStore } from "@/stores/tags";
import { offerColumns } from "@/features/offers/offer-columns";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";

export function OfferList() {
  const { t } = useTranslation(["offers", "common"]);
  const offersQuery = useOffers();
  const networksQuery = useNetworks();
  const assignments = useTagsStore((s) => s.assignments);

  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const networks = networksQuery.data?.networks ?? [];
  const networkNameById = React.useMemo(
    () => Object.fromEntries((networksQuery.data?.networks ?? []).map((n) => [n.id, n.name])),
    [networksQuery.data],
  );
  const filtered = React.useMemo(
    () => filterByTags("offer", offersQuery.data?.offers ?? [], tagFilter, assignments),
    [offersQuery.data, tagFilter, assignments],
  );
  const columns = React.useMemo(() => offerColumns(t, networkNameById), [t, networkNameById]);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "offers" })}</h1>
      {networks.length === 0 ? (
        <Button disabled>
          <PlusIcon className="size-4" />
          {t("list.newButton", { ns: "offers" })}
        </Button>
      ) : (
        <Button asChild>
          <Link href="/offers/new">
            <PlusIcon className="size-4" />
            {t("list.newButton", { ns: "offers" })}
          </Link>
        </Button>
      )}
    </div>
  );

  if (offersQuery.isPending || networksQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label={t("list.loading", { ns: "offers" })} />
      </div>
    );
  }

  if (offersQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadError", { ns: "offers" })}
          description={offersQuery.error.message}
          onRetry={() => offersQuery.refetch()}
        />
      </div>
    );
  }
  if (networksQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title={t("list.loadNetworksError", { ns: "offers" })}
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
        searchPlaceholder={t("list.searchPlaceholder", { ns: "offers" })}
        emptyTitle={t("list.emptyTitle", { ns: "offers" })}
        emptyDescription={
          networks.length === 0
            ? t("list.emptyDescriptionNoNetworks", { ns: "offers" })
            : t("list.emptyDescription", { ns: "offers" })
        }
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
            <TagIcon className="size-3.5" /> {t("list.editTagsButton", { ns: "offers" })}
          </Button>
        )}
      />

      {bulkTarget && (
        <BulkTagDialog
          entityType="offer"
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

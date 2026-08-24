"use client";

import * as React from "react";
import { useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCreatePwa, usePwas, useUpdatePwa } from "@/hooks/use-pwas";
import { useTagsStore } from "@/stores/tags";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { pwaColumns } from "@/features/pwa/pwa-columns";
import { PwaFormSheet, type PwaFormValues } from "@/features/pwa/pwa-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Pwa } from "@/lib/api/pwa";

function toFormValues(pwa: Pwa): PwaFormValues {
  return {
    name: pwa.name,
    shortName: pwa.shortName,
    themeColor: pwa.themeColor,
    backgroundColor: pwa.backgroundColor,
    iconUrl: pwa.iconUrl,
    startUrl: pwa.startUrl,
    bounceInAppWebview: pwa.bounceInAppWebview,
    status: pwa.status,
  };
}

export function PwaList() {
  const { t } = useTranslation(["pwa", "common"]);
  const pwasQuery = usePwas();
  const assignments = useTagsStore((s) => s.assignments);
  const searchParams = useSearchParams();
  const galleryItem = useContentGalleryStore((s) => s.items.find((i) => i.id === searchParams.get("gallery")));

  const [target, setTarget] = React.useState<Pwa | null | undefined>(() => (galleryItem?.pwaPayload ? null : undefined));
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => pwaColumns(t, (pwa) => setTarget(pwa)), [t]);
  const filtered = React.useMemo(
    () => filterByTags("pwa", pwasQuery.data?.pwas ?? [], tagFilter, assignments),
    [pwasQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "pwa" })}</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        {t("list.newButton", { ns: "pwa" })}
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

      {target !== undefined && (
        <PwaFormDialog
          key={target?.id ?? "new"}
          target={target}
          galleryTitle={galleryItem?.title}
          galleryDefaults={target ? undefined : galleryItem?.pwaPayload}
          onClose={() => setTarget(undefined)}
        />
      )}

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

function PwaFormDialog({
  target,
  galleryTitle,
  galleryDefaults,
  onClose,
}: {
  target: Pwa | null;
  galleryTitle?: string;
  galleryDefaults?: Partial<PwaFormValues>;
  onClose: () => void;
}) {
  const { t } = useTranslation("pwa");
  const createPwa = useCreatePwa();
  const updatePwa = useUpdatePwa(target?.id ?? "");

  function handleSubmit(values: PwaFormValues) {
    if (target) {
      updatePwa.mutate(values, {
        onSuccess: () => {
          toast(t("toast.updated"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      });
    } else {
      createPwa.mutate(values, {
        onSuccess: () => {
          toast(t("toast.created"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      });
    }
  }

  return (
    <PwaFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        target
          ? t("form.titleEdit", { name: target.name })
          : galleryTitle
            ? t("form.titleFromGallery", { title: galleryTitle })
            : t("form.titleNew")
      }
      submitLabel={target ? t("form.submitEdit") : t("form.submitCreate")}
      defaultValues={target ? toFormValues(target) : galleryDefaults ? { name: galleryTitle, ...galleryDefaults } : {}}
      onSubmit={handleSubmit}
    />
  );
}

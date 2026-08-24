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
import { useCreateLanding, useLandings, useUpdateLanding } from "@/hooks/use-landings";
import { useTagsStore } from "@/stores/tags";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { landingColumns } from "@/features/landings/landing-columns";
import { LandingFormSheet, type LandingFormValues } from "@/features/landings/landing-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Landing } from "@/lib/api/landings";

function toFormValues(landing: Landing): LandingFormValues {
  return {
    name: landing.name,
    type: landing.type,
    url: landing.url,
    content: landing.content,
    status: landing.status,
  };
}

export function LandingList() {
  const { t } = useTranslation(["landings", "common"]);
  const landingsQuery = useLandings();
  const assignments = useTagsStore((s) => s.assignments);
  const searchParams = useSearchParams();
  const galleryItem = useContentGalleryStore((s) => s.items.find((i) => i.id === searchParams.get("gallery")));

  const [target, setTarget] = React.useState<Landing | null | undefined>(() => (galleryItem?.landingPayload ? null : undefined));
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const columns = React.useMemo(() => landingColumns(t, (landing) => setTarget(landing)), [t]);
  const filtered = React.useMemo(
    () => filterByTags("landing", landingsQuery.data?.landings ?? [], tagFilter, assignments),
    [landingsQuery.data, tagFilter, assignments],
  );

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">{t("list.title", { ns: "landings" })}</h1>
      <Button onClick={() => setTarget(null)}>
        <PlusIcon className="size-4" />
        {t("list.newButton", { ns: "landings" })}
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

      {target !== undefined && (
        <LandingFormDialog
          key={target?.id ?? "new"}
          target={target}
          galleryTitle={galleryItem?.title}
          galleryDefaults={target ? undefined : galleryItem?.landingPayload}
          onClose={() => setTarget(undefined)}
        />
      )}

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

function LandingFormDialog({
  target,
  galleryTitle,
  galleryDefaults,
  onClose,
}: {
  target: Landing | null;
  galleryTitle?: string;
  galleryDefaults?: Partial<LandingFormValues>;
  onClose: () => void;
}) {
  const { t } = useTranslation("landings");
  const createLanding = useCreateLanding();
  const updateLanding = useUpdateLanding(target?.id ?? "");

  function handleSubmit(values: LandingFormValues) {
    const input = {
      name: values.name,
      type: values.type,
      status: values.status,
      // `url` is ignored server-side for `internal` (recomputed from
      // `name`); only sent for `external`, where it's the real value.
      url: values.type === "external" ? (values.url ?? "") : undefined,
      content: values.type === "internal" ? (values.content ?? "") : "",
    };
    if (target) {
      updateLanding.mutate(input, {
        onSuccess: () => {
          toast(t("toast.updated"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
      });
    } else {
      createLanding.mutate(input, {
        onSuccess: () => {
          toast(t("toast.created"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      });
    }
  }

  return (
    <LandingFormSheet
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

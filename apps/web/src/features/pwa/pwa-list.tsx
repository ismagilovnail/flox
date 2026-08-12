"use client";

import * as React from "react";
import { useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { usePwasStore } from "@/stores/pwas";
import { useTagsStore } from "@/stores/tags";
import { useContentGalleryStore } from "@/stores/content-gallery";
import { pwaColumns } from "@/features/pwa/pwa-columns";
import { PwaFormSheet, type PwaFormValues } from "@/features/pwa/pwa-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Pwa } from "@/lib/mock/pwas";

export function PwaList() {
  const pwas = usePwasStore((s) => s.pwas);
  const addPwa = usePwasStore((s) => s.addPwa);
  const updatePwa = usePwasStore((s) => s.updatePwa);
  const assignments = useTagsStore((s) => s.assignments);
  const searchParams = useSearchParams();
  const galleryItem = useContentGalleryStore((s) => s.items.find((i) => i.id === searchParams.get("gallery")));

  const [target, setTarget] = React.useState<Pwa | null | undefined>(() => (galleryItem?.pwaPayload ? null : undefined));
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  function handleSubmit(values: PwaFormValues) {
    if (target) {
      updatePwa(target.id, values);
      toast("PWA updated", { description: values.name });
    } else {
      addPwa(values);
      toast("PWA created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => pwaColumns((pwa) => setTarget(pwa)), []);
  const filtered = React.useMemo(
    () => filterByTags("pwa", pwas, tagFilter, assignments),
    [pwas, tagFilter, assignments],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">PWA</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New PWA
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder="Search PWAs..."
        emptyTitle="No PWAs yet"
        emptyDescription="Add a PWA manifest to use as the installable step of a flow."
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
        <PwaFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : galleryItem ? `New PWA — from ${galleryItem.title}` : "New PWA"}
          submitLabel={target ? "Save changes" : "Create PWA"}
          defaultValues={target ?? (galleryItem?.pwaPayload ? { name: galleryItem.title, ...galleryItem.pwaPayload } : {})}
          onSubmit={handleSubmit}
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

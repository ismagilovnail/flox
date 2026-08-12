"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useLandingsStore } from "@/stores/landings";
import { useTagsStore } from "@/stores/tags";
import { slugify } from "@/lib/utils";
import { landingColumns } from "@/features/landings/landing-columns";
import { LandingFormSheet, type LandingFormValues } from "@/features/landings/landing-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Landing } from "@/lib/mock/landings";

export function LandingList() {
  const landings = useLandingsStore((s) => s.landings);
  const addLanding = useLandingsStore((s) => s.addLanding);
  const updateLanding = useLandingsStore((s) => s.updateLanding);
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<Landing | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  function handleSubmit(values: LandingFormValues) {
    const input = {
      name: values.name,
      type: values.type,
      status: values.status,
      url: values.type === "internal" ? `https://cdn.floxlink.io/lnd/${slugify(values.name)}` : (values.url ?? ""),
      content: values.type === "internal" ? (values.content ?? "") : "",
    };
    if (target) {
      updateLanding(target.id, input);
      toast("Landing updated", { description: values.name });
    } else {
      addLanding(input);
      toast("Landing created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(() => landingColumns((landing) => setTarget(landing)), []);
  const filtered = React.useMemo(
    () => filterByTags("landing", landings, tagFilter, assignments),
    [landings, tagFilter, assignments],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Landings</h1>
        <Button onClick={() => setTarget(null)}>
          <PlusIcon className="size-4" />
          New Landing
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder="Search landings..."
        emptyTitle="No landings yet"
        emptyDescription="Add a landing page to use as the first step of a flow."
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
        <LandingFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Landing"}
          submitLabel={target ? "Save changes" : "Create landing"}
          defaultValues={target ?? {}}
          onSubmit={handleSubmit}
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

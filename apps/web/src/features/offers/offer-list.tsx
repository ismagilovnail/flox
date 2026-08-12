"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { useOffersStore } from "@/stores/offers";
import { useNetworksStore } from "@/stores/networks";
import { useTagsStore } from "@/stores/tags";
import { offerColumns } from "@/features/offers/offer-columns";
import { OfferFormSheet, type OfferFormValues } from "@/features/offers/offer-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Offer } from "@/lib/mock/offers";

function toFormValues(offer: Offer): OfferFormValues {
  return {
    networkId: offer.networkId,
    name: offer.name,
    countries: offer.countries,
    payout: offer.payout,
    currency: offer.currency,
    cap: offer.cap === null ? "" : String(offer.cap),
    status: offer.status,
    links: offer.links,
  };
}

export function OfferList() {
  const offers = useOffersStore((s) => s.offers);
  const addOffer = useOffersStore((s) => s.addOffer);
  const updateOffer = useOffersStore((s) => s.updateOffer);
  const networks = useNetworksStore((s) => s.networks);
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<Offer | null | undefined>(undefined);
  const [tagFilter, setTagFilter] = React.useState<string[]>([]);
  const [bulkTarget, setBulkTarget] = React.useState<{ ids: string[]; clear: () => void } | null>(null);

  const networkNameById = React.useMemo(
    () => Object.fromEntries(networks.map((n) => [n.id, n.name])),
    [networks],
  );
  const filtered = React.useMemo(
    () => filterByTags("offer", offers, tagFilter, assignments),
    [offers, tagFilter, assignments],
  );

  function handleSubmit(values: OfferFormValues) {
    const input = { ...values, cap: values.cap.trim() === "" ? null : Number(values.cap) };
    if (target) {
      updateOffer(target.id, input);
      toast("Offer updated", { description: values.name });
    } else {
      addOffer(input);
      toast("Offer created", { description: values.name });
    }
    setTarget(undefined);
  }

  const columns = React.useMemo(
    () => offerColumns((offer) => setTarget(offer), networkNameById),
    [networkNameById],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Offers</h1>
        <Button onClick={() => setTarget(null)} disabled={networks.length === 0}>
          <PlusIcon className="size-4" />
          New Offer
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        searchPlaceholder="Search offers..."
        emptyTitle="No offers yet"
        emptyDescription="Add an offer under one of your networks to route traffic to it from a Flow."
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
        <OfferFormSheet
          key={target?.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(undefined)}
          title={target ? `Edit ${target.name}` : "New Offer"}
          submitLabel={target ? "Save changes" : "Create offer"}
          defaultValues={target ? toFormValues(target) : {}}
          networks={networks}
          onSubmit={handleSubmit}
        />
      )}

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

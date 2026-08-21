"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon, TagIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { useCreateOffer, useOffers, useUpdateOffer } from "@/hooks/use-offers";
import { useNetworks } from "@/hooks/use-networks";
import { useTagsStore } from "@/stores/tags";
import { offerColumns } from "@/features/offers/offer-columns";
import { OfferFormSheet, type OfferFormValues } from "@/features/offers/offer-form-sheet";
import { TagFilterControl } from "@/features/tags/tag-filter-control";
import { BulkTagDialog } from "@/features/tags/bulk-tag-dialog";
import { filterByTags } from "@/features/tags/filter-by-tags";
import type { Offer } from "@/lib/api/offers";
import type { Network } from "@/lib/api/networks";

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
  const offersQuery = useOffers();
  const networksQuery = useNetworks();
  const assignments = useTagsStore((s) => s.assignments);

  const [target, setTarget] = React.useState<Offer | null | undefined>(undefined);
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
  const columns = React.useMemo(() => offerColumns((offer) => setTarget(offer), networkNameById), [networkNameById]);

  const header = (
    <div className="flex items-center justify-between">
      <h1 className="text-2xl font-semibold tracking-tight">Offers</h1>
      <Button onClick={() => setTarget(null)} disabled={networks.length === 0}>
        <PlusIcon className="size-4" />
        New Offer
      </Button>
    </div>
  );

  if (offersQuery.isPending || networksQuery.isPending) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <LoadingState label="Loading offers…" />
      </div>
    );
  }

  if (offersQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState title="Couldn't load offers" description={offersQuery.error.message} onRetry={() => offersQuery.refetch()} />
      </div>
    );
  }
  if (networksQuery.isError) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <ErrorState
          title="Couldn't load networks"
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
        searchPlaceholder="Search offers..."
        emptyTitle="No offers yet"
        emptyDescription={
          networks.length === 0
            ? "Add a network first — an offer always belongs to one."
            : "Add an offer under one of your networks to route traffic to it from a Flow."
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
            <TagIcon className="size-3.5" /> Edit Tags
          </Button>
        )}
      />

      {target !== undefined && (
        <OfferFormDialog key={target?.id ?? "new"} target={target} networks={networks} onClose={() => setTarget(undefined)} />
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

function OfferFormDialog({
  target,
  networks,
  onClose,
}: {
  target: Offer | null;
  networks: Network[];
  onClose: () => void;
}) {
  const createOffer = useCreateOffer();
  const updateOffer = useUpdateOffer(target?.id ?? "");

  function handleSubmit(values: OfferFormValues) {
    const cap = values.cap.trim() === "" ? null : Number(values.cap);
    const links = values.links.map((l) => ({ label: l.label, url: l.url }));

    if (target) {
      updateOffer.mutate(
        {
          networkId: values.networkId,
          name: values.name,
          countries: values.countries,
          payout: values.payout,
          currency: values.currency,
          cap,
          status: values.status,
          links,
        },
        {
          onSuccess: () => {
            toast("Offer updated", { description: values.name });
            onClose();
          },
          onError: (err) => toast.error("Couldn't update offer", { description: err.message }),
        },
      );
    } else {
      createOffer.mutate(
        {
          networkId: values.networkId,
          name: values.name,
          countries: values.countries,
          payout: values.payout,
          currency: values.currency,
          cap,
          links,
        },
        {
          onSuccess: () => {
            toast("Offer created", { description: values.name });
            onClose();
          },
          onError: (err) => toast.error("Couldn't create offer", { description: err.message }),
        },
      );
    }
  }

  return (
    <OfferFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={target ? `Edit ${target.name}` : "New Offer"}
      submitLabel={target ? "Save changes" : "Create offer"}
      showStatus={!!target}
      defaultValues={target ? toFormValues(target) : {}}
      networks={networks}
      onSubmit={handleSubmit}
    />
  );
}

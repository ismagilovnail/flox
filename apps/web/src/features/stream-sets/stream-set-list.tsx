"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";

import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { emptyGroup } from "@/lib/filters";
import { dehydrateFilterNode, type CreateStreamSetInput, type StreamSet } from "@/lib/api/stream-sets";
import {
  useCreateStreamSet,
  useDeleteStreamSet,
  useDuplicateStreamSet,
  useReorderStreamSets,
  useStreamSets,
  useUpdateStreamSet,
} from "@/hooks/use-stream-sets";
import { useNetworks } from "@/hooks/use-networks";
import { useOffers } from "@/hooks/use-offers";
import { useLandings } from "@/hooks/use-landings";
import { usePwas } from "@/hooks/use-pwas";
import { usePostlandings } from "@/hooks/use-postlandings";
import { usePixels } from "@/hooks/use-pixels";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import type { Landing } from "@/lib/api/landings";
import type { Pwa } from "@/lib/api/pwa";
import type { Postlanding } from "@/lib/api/postlanding";
import type { Pixel } from "@/lib/api/pixels";
import { StreamSetRow } from "@/features/stream-sets/stream-set-row";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";

// firstOffer must be a real offer — the backend rejects an empty/unknown
// offerId (streamset.Service's own "offer %q not found in this
// organization" check), so creating with a placeholder empty string
// 404s. Defaulting to the org's first offer, editable immediately in the
// row that appears, is what makes "click New, it just appears" possible
// at all instead of needing a dialog to collect a real one first.
function emptyStreamSetForm(firstOffer: Offer): CreateStreamSetInput {
  return {
    name: "New stream set",
    fallbackUrl: "",
    rootFilter: dehydrateFilterNode(emptyGroup()),
    flows: [
      {
        name: "Primary offer",
        active: true,
        weight: 100,
        landing: { enabled: false, landingId: "", asPwa: false },
        pwa: { enabled: false, pwaId: "", pwaType: "", url: "" },
        postlanding: { enabled: false, postlandingId: "" },
        destination: { kind: "offer", networkId: firstOffer.networkId, offerId: firstOffer.id },
      },
    ],
    pixelIds: [],
  };
}

function toCreateInput(values: StreamSetFormValues): CreateStreamSetInput {
  return {
    name: values.name,
    fallbackUrl: values.fallbackUrl,
    rootFilter: dehydrateFilterNode(values.rootFilter),
    flows: values.flows.map(({ name, active, weight, landing, pwa, postlanding, destination }) => ({
      name,
      active,
      weight,
      landing,
      pwa,
      postlanding,
      destination,
    })),
    pixelIds: values.pixelIds,
  };
}

export function StreamSetList({
  campaignId,
  disabled = false,
}: {
  campaignId: string;
  /** Archived campaigns don't route traffic — routing config edits are
   * locked instead of letting them look saved while they can't matter. */
  disabled?: boolean;
}) {
  const { t } = useTranslation("streamSets");
  const streamSetsQuery = useStreamSets(campaignId);
  const networksQuery = useNetworks();
  const offersQuery = useOffers();
  const landingsQuery = useLandings();
  const pwasQuery = usePwas();
  const postlandingsQuery = usePostlandings();
  const pixelsQuery = usePixels();
  const reorder = useReorderStreamSets(campaignId);
  const createStreamSet = useCreateStreamSet(campaignId);

  const [search, setSearch] = React.useState("");

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const streamSets = streamSetsQuery.data?.streamSets ?? [];
  const networks = networksQuery.data?.networks ?? [];
  const offers = offersQuery.data?.offers ?? [];
  const landings = landingsQuery.data?.landings ?? [];
  const pwas = pwasQuery.data?.pwas ?? [];
  const postlandings = postlandingsQuery.data?.postlandings ?? [];
  const pixels = pixelsQuery.data?.pixels ?? [];

  const trimmedSearch = search.trim().toLowerCase();
  const filteredStreamSets = trimmedSearch
    ? streamSets.filter((s) => s.name.toLowerCase().includes(trimmedSearch))
    : streamSets;

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    // Reordering a filtered subset would silently reorder the full list
    // underneath it (drag positions are indices into filteredStreamSets,
    // not streamSets) — sensors are disabled while a search is active
    // (see below) so this shouldn't fire, but guard anyway.
    if (trimmedSearch || !over || active.id === over.id) return;
    const ids = streamSets.map((s) => s.id);
    const oldIndex = ids.indexOf(String(active.id));
    const newIndex = ids.indexOf(String(over.id));
    const reordered = [...ids];
    reordered.splice(oldIndex, 1);
    reordered.splice(newIndex, 0, String(active.id));
    reorder.mutate(reordered, {
      onError: (err) => toast.error(t("list.reorderError"), { description: err.message }),
    });
  }

  // "New stream set" creates immediately (no dialog) — the row appears
  // collapsed in the list, ready for the operator to click into and edit,
  // matching the reference layout this was built to match. Needs a real
  // offer to default the flow's destination to (see emptyStreamSetForm's
  // own comment) — disabled with an explanatory description, same as the
  // pre-existing networks.length===0 case, when the org has none yet.
  function handleAddStreamSet() {
    if (!offers[0]) return;
    createStreamSet.mutate(emptyStreamSetForm(offers[0]), {
      onSuccess: (created) => toast(t("toast.created"), { description: created.name }),
      onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
    });
  }

  const header = (
    <CardHeader>
      <CardTitle>{t("list.title")}</CardTitle>
      <CardDescription>{t("list.description")}</CardDescription>
      <CardAction>
        <Button
          size="sm"
          onClick={handleAddStreamSet}
          disabled={disabled || networks.length === 0 || offers.length === 0 || createStreamSet.isPending}
        >
          <PlusIcon className="size-4" /> {t("list.newButton")}
        </Button>
      </CardAction>
    </CardHeader>
  );

  if (
    streamSetsQuery.isPending ||
    networksQuery.isPending ||
    offersQuery.isPending ||
    landingsQuery.isPending ||
    pwasQuery.isPending ||
    postlandingsQuery.isPending ||
    pixelsQuery.isPending
  ) {
    return (
      <Card>
        {header}
        <CardContent>
          <LoadingState label={t("list.loading")} />
        </CardContent>
      </Card>
    );
  }

  if (streamSetsQuery.isError) {
    return (
      <Card>
        {header}
        <CardContent>
          <ErrorState
            title={t("list.loadError")}
            description={streamSetsQuery.error.message}
            onRetry={() => streamSetsQuery.refetch()}
          />
        </CardContent>
      </Card>
    );
  }
  if (
    networksQuery.isError ||
    offersQuery.isError ||
    landingsQuery.isError ||
    pwasQuery.isError ||
    postlandingsQuery.isError ||
    pixelsQuery.isError
  ) {
    const err =
      networksQuery.error ?? offersQuery.error ?? landingsQuery.error ?? pwasQuery.error ?? postlandingsQuery.error ?? pixelsQuery.error;
    return (
      <Card>
        {header}
        <CardContent>
          <ErrorState
            title={t("list.loadNetworksOffersError")}
            description={err?.message}
            onRetry={() => {
              networksQuery.refetch();
              offersQuery.refetch();
              landingsQuery.refetch();
              pwasQuery.refetch();
              postlandingsQuery.refetch();
              pixelsQuery.refetch();
            }}
          />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      {header}
      <CardContent className="flex flex-col gap-4">
        {streamSets.length === 0 ? (
          <EmptyState title={t("list.emptyTitle")} description={t("list.emptyDescription")} />
        ) : (
          <>
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("list.searchPlaceholder")}
              className="max-w-sm"
            />

            {filteredStreamSets.length === 0 ? (
              <EmptyState title={t("list.noResultsTitle")} description={t("list.noResultsDescription")} />
            ) : (
              <DndContext
                id={`stream-sets-${campaignId}`}
                sensors={trimmedSearch ? [] : sensors}
                collisionDetection={closestCenter}
                onDragEnd={handleDragEnd}
              >
                <SortableContext items={filteredStreamSets.map((s) => s.id)} strategy={verticalListSortingStrategy}>
                  <div className="flex flex-col gap-2">
                    {filteredStreamSets.map((streamSet) => (
                      <StreamSetRowContainer
                        key={streamSet.id}
                        streamSet={streamSet}
                        networks={networks}
                        offers={offers}
                        landings={landings}
                        pwas={pwas}
                        postlandings={postlandings}
                        pixels={pixels}
                        campaignId={campaignId}
                        disabled={disabled}
                      />
                    ))}
                  </div>
                </SortableContext>
              </DndContext>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

/** Each row owns its own update/duplicate/delete mutations so a toast/
 * error from one row's action can't be misattributed to another mid-list.
 * Status toggling and the inline editor's full-form save both PATCH
 * through the same useUpdateStreamSet mutation — one status field or
 * every field, same endpoint. */
function StreamSetRowContainer({
  streamSet,
  networks,
  offers,
  landings,
  pwas,
  postlandings,
  pixels,
  campaignId,
  disabled,
}: {
  streamSet: StreamSet;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  pixels: Pixel[];
  campaignId: string;
  disabled: boolean;
}) {
  const { t } = useTranslation("streamSets");
  const update = useUpdateStreamSet(campaignId, streamSet.id);
  const duplicate = useDuplicateStreamSet(campaignId);
  const del = useDeleteStreamSet(campaignId);

  return (
    <StreamSetRow
      streamSet={streamSet}
      networks={networks}
      offers={offers}
      landings={landings}
      pwas={pwas}
      postlandings={postlandings}
      pixels={pixels}
      disabled={disabled}
      onSubmit={(values) =>
        update.mutate(
          { ...toCreateInput(values), status: values.status },
          {
            onSuccess: () => toast(t("toast.updated"), { description: values.name }),
            onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
          },
        )
      }
      onDuplicate={() =>
        duplicate.mutate(streamSet.id, {
          onSuccess: () => toast(t("toast.duplicated"), { description: t("form.flowCopySuffix", { name: streamSet.name }) }),
          onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
        })
      }
      onDelete={() =>
        del.mutate(streamSet.id, {
          onSuccess: () => toast(t("toast.removed"), { description: streamSet.name }),
          onError: (err) => toast.error(t("toast.removeError"), { description: err.message }),
        })
      }
      onToggleStatus={() =>
        update.mutate(
          { status: streamSet.status === "active" ? "paused" : "active" },
          { onError: (err) => toast.error(t("toast.updateError"), { description: err.message }) },
        )
      }
    />
  );
}

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
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { genId } from "@/lib/id";
import { emptyGroup } from "@/lib/filters";
import {
  dehydrateFilterNode,
  hydrateRootFilter,
  type CreateStreamSetInput,
  type StreamSet,
} from "@/lib/api/stream-sets";
import {
  useCreateStreamSet,
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
import { StreamSetFormSheet } from "@/features/stream-sets/stream-set-form-sheet";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";

function emptyStreamSetForm(firstNetworkId: string): StreamSetFormValues {
  return {
    name: "",
    status: "active",
    rootFilter: emptyGroup(),
    flows: [
      {
        id: genId(),
        name: "Primary offer",
        active: true,
        weight: 100,
        landing: { enabled: false, landingId: "", asPwa: false },
        pwa: { enabled: false, pwaId: "", pwaType: "" },
        postlanding: { enabled: false, postlandingId: "" },
        destination: { kind: "offer", networkId: firstNetworkId, offerId: "" },
      },
    ],
    pixelIds: [],
    fallbackUrl: "",
  };
}

function toFormValues(streamSet: StreamSet): StreamSetFormValues {
  return {
    name: streamSet.name,
    status: streamSet.status,
    rootFilter: hydrateRootFilter(streamSet.rootFilter),
    flows: streamSet.flows,
    pixelIds: streamSet.pixelIds,
    fallbackUrl: streamSet.fallbackUrl,
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

export function StreamSetList({ campaignId }: { campaignId: string }) {
  const { t } = useTranslation("streamSets");
  const streamSetsQuery = useStreamSets(campaignId);
  const networksQuery = useNetworks();
  const offersQuery = useOffers();
  const landingsQuery = useLandings();
  const pwasQuery = usePwas();
  const postlandingsQuery = usePostlandings();
  const pixelsQuery = usePixels();
  const reorder = useReorderStreamSets(campaignId);

  const [target, setTarget] = React.useState<{ id: string | null } | null>(null);

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

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
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

  const editingStreamSet = target?.id ? streamSets.find((s) => s.id === target.id) : undefined;

  const header = (
    <CardHeader>
      <CardTitle>{t("list.title")}</CardTitle>
      <CardDescription>{t("list.description")}</CardDescription>
      <CardAction>
        <Button size="sm" onClick={() => setTarget({ id: null })} disabled={networks.length === 0}>
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
      <CardContent>
        {streamSets.length === 0 ? (
          <EmptyState title={t("list.emptyTitle")} description={t("list.emptyDescription")} />
        ) : (
          <DndContext id={`stream-sets-${campaignId}`} sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={streamSets.map((s) => s.id)} strategy={verticalListSortingStrategy}>
              <div className="flex flex-col gap-2">
                {streamSets.map((streamSet) => (
                  <StreamSetRowContainer
                    key={streamSet.id}
                    streamSet={streamSet}
                    offers={offers}
                    onEdit={() => setTarget({ id: streamSet.id })}
                    campaignId={campaignId}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        )}
      </CardContent>

      {target && (
        <StreamSetFormDialog
          key={target.id ?? "new"}
          target={editingStreamSet ?? null}
          networks={networks}
          offers={offers}
          landings={landings}
          pwas={pwas}
          postlandings={postlandings}
          pixels={pixels}
          campaignId={campaignId}
          firstNetworkId={networks[0]?.id ?? ""}
          onClose={() => setTarget(null)}
        />
      )}
    </Card>
  );
}

/** Each row owns its own duplicate mutation so a toast/error from one
 * row's action can't be misattributed to another mid-list. Status
 * toggling PATCHes {status} only — the same partial-update convention
 * every other domain's archive/pause action uses this session. */
function StreamSetRowContainer({
  streamSet,
  offers,
  campaignId,
  onEdit,
}: {
  streamSet: StreamSet;
  offers: Offer[];
  campaignId: string;
  onEdit: () => void;
}) {
  const { t } = useTranslation("streamSets");
  const updateStatus = useUpdateStreamSet(campaignId, streamSet.id);
  const duplicate = useDuplicateStreamSet(campaignId);

  return (
    <StreamSetRow
      streamSet={streamSet}
      offers={offers}
      onEdit={onEdit}
      onDuplicate={() =>
        duplicate.mutate(streamSet.id, {
          onSuccess: () => toast(t("toast.duplicated"), { description: t("form.flowCopySuffix", { name: streamSet.name }) }),
          onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
        })
      }
      onToggleStatus={() =>
        updateStatus.mutate(
          { status: streamSet.status === "active" ? "paused" : "active" },
          { onError: (err) => toast.error(t("toast.updateError"), { description: err.message }) },
        )
      }
    />
  );
}

function StreamSetFormDialog({
  target,
  networks,
  offers,
  landings,
  pwas,
  postlandings,
  pixels,
  campaignId,
  firstNetworkId,
  onClose,
}: {
  target: StreamSet | null;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  pixels: Pixel[];
  campaignId: string;
  firstNetworkId: string;
  onClose: () => void;
}) {
  const { t } = useTranslation("streamSets");
  const createStreamSet = useCreateStreamSet(campaignId);
  const updateStreamSet = useUpdateStreamSet(campaignId, target?.id ?? "");

  function handleSubmit(values: StreamSetFormValues) {
    const input = toCreateInput(values);
    if (target) {
      updateStreamSet.mutate(
        { ...input, status: values.status },
        {
          onSuccess: () => {
            toast(t("toast.updated"), { description: values.name });
            onClose();
          },
          onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
        },
      );
    } else {
      createStreamSet.mutate(input, {
        onSuccess: () => {
          toast(t("toast.created"), { description: values.name });
          onClose();
        },
        onError: (err) => toast.error(t("toast.createError"), { description: err.message }),
      });
    }
  }

  return (
    <StreamSetFormSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={target ? t("form.titleEdit", { name: target.name }) : t("form.titleNew")}
      submitLabel={target ? t("form.submitEdit") : t("form.submitCreate")}
      defaultValues={target ? toFormValues(target) : emptyStreamSetForm(firstNetworkId)}
      networks={networks}
      offers={offers}
      landings={landings}
      pwas={pwas}
      postlandings={postlandings}
      pixels={pixels}
      onSubmit={handleSubmit}
    />
  );
}

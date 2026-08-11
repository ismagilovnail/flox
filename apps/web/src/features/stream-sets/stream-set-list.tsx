"use client";

import * as React from "react";
import { toast } from "sonner";
import { PlusIcon } from "lucide-react";
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
import { useStreamSetsStore } from "@/stores/stream-sets";
import { genId } from "@/lib/id";
import { emptyGroup } from "@/lib/filters";
import { NETWORKS } from "@/lib/mock/flow-entities";
import { type StreamSet } from "@/lib/mock/stream-sets";
import { StreamSetRow } from "@/features/stream-sets/stream-set-row";
import { StreamSetFormSheet } from "@/features/stream-sets/stream-set-form-sheet";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";

function emptyStreamSetForm(): StreamSetFormValues {
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
        pwa: { enabled: false, pwaId: "", pwaType: "internal" },
        postlanding: { enabled: false, postlandingId: "" },
        destination: { kind: "offer", networkId: NETWORKS[0].id, offerId: "", offerUrl: "" },
      },
    ],
    pixels: [],
    fallbackUrl: "",
  };
}

function toFormValues(streamSet: StreamSet): StreamSetFormValues {
  return {
    name: streamSet.name,
    status: streamSet.status,
    rootFilter: streamSet.rootFilter,
    flows: streamSet.flows,
    pixels: streamSet.pixels.map((url) => ({ id: genId(), url })),
    fallbackUrl: streamSet.fallbackUrl,
  };
}

export function StreamSetList({ campaignId }: { campaignId: string }) {
  const streamSets = useStreamSetsStore((s) => s.listByCampaign(campaignId));
  const addStreamSet = useStreamSetsStore((s) => s.addStreamSet);
  const updateStreamSet = useStreamSetsStore((s) => s.updateStreamSet);
  const setStatus = useStreamSetsStore((s) => s.setStatus);
  const duplicateStreamSet = useStreamSetsStore((s) => s.duplicateStreamSet);
  const reorder = useStreamSetsStore((s) => s.reorder);

  const [target, setTarget] = React.useState<{ id: string | null } | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const ids = streamSets.map((s) => s.id);
    const oldIndex = ids.indexOf(String(active.id));
    const newIndex = ids.indexOf(String(over.id));
    const reordered = [...ids];
    reordered.splice(oldIndex, 1);
    reordered.splice(newIndex, 0, String(active.id));
    reorder(campaignId, reordered);
  }

  function handleSubmit(values: StreamSetFormValues) {
    const input = { ...values, pixels: values.pixels.map((p) => p.url) };
    if (target?.id) {
      updateStreamSet(campaignId, target.id, input);
      toast("Stream set updated", { description: values.name });
    } else {
      addStreamSet(campaignId, input);
      toast("Stream set created", { description: values.name });
    }
    setTarget(null);
  }

  const editingStreamSet = target?.id ? streamSets.find((s) => s.id === target.id) : undefined;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Stream Sets</CardTitle>
        <CardDescription>
          Evaluated top-to-bottom by priority — the first set whose filters match wins. No match falls back to the
          campaign fallback in Settings.
        </CardDescription>
        <CardAction>
          <Button size="sm" onClick={() => setTarget({ id: null })}>
            <PlusIcon className="size-4" /> New Stream Set
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        {streamSets.length === 0 ? (
          <EmptyState
            title="No stream sets yet"
            description="All traffic falls through to the campaign fallback URL until you add one."
          />
        ) : (
          <DndContext id={`stream-sets-${campaignId}`} sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={streamSets.map((s) => s.id)} strategy={verticalListSortingStrategy}>
              <div className="flex flex-col gap-2">
                {streamSets.map((streamSet) => (
                  <StreamSetRow
                    key={streamSet.id}
                    streamSet={streamSet}
                    onEdit={() => setTarget({ id: streamSet.id })}
                    onDuplicate={() => {
                      duplicateStreamSet(campaignId, streamSet.id);
                      toast("Stream set duplicated", { description: `${streamSet.name} (Copy)` });
                    }}
                    onToggleStatus={() =>
                      setStatus(campaignId, streamSet.id, streamSet.status === "active" ? "paused" : "active")
                    }
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        )}
      </CardContent>

      {target && (
        <StreamSetFormSheet
          key={target.id ?? "new"}
          open
          onOpenChange={(open) => !open && setTarget(null)}
          title={editingStreamSet ? `Edit ${editingStreamSet.name}` : "New Stream Set"}
          submitLabel={editingStreamSet ? "Save changes" : "Create stream set"}
          defaultValues={editingStreamSet ? toFormValues(editingStreamSet) : emptyStreamSetForm()}
          onSubmit={handleSubmit}
        />
      )}
    </Card>
  );
}

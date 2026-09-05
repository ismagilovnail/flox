"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, MoreHorizontalIcon, PauseIcon, PencilIcon, PlayIcon, SendIcon, Trash2Icon } from "lucide-react";

import { IconButton } from "@/components/ui/icon-button";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ApiError } from "@/lib/api/client";
import {
  useDeleteCustomMetric,
  useDuplicateCustomMetric,
  useSetCustomMetricActive,
  useUpdateCustomMetric,
} from "@/hooks/use-custom-metrics";
import type { CustomMetric } from "@/lib/custom-metrics";

export function CustomMetricRowActions({ metric, onEdit }: { metric: CustomMetric; onEdit: () => void }) {
  const updateMetric = useUpdateCustomMetric();
  const setActive = useSetCustomMetricActive();
  const deleteMetric = useDeleteCustomMetric();
  const duplicateMetric = useDuplicateCustomMetric();
  const [confirmDelete, setConfirmDelete] = React.useState(false);

  function togglePublish() {
    const next = metric.status === "published" ? "draft" : "published";
    // Targets are cleared server-side on a publish -> draft transition
    // (apps/internal/custommetrics.Service.Update's own rule) — no need
    // to compute that here.
    updateMetric.mutate(
      { id: metric.id, input: { status: next } },
      {
        onSuccess: () =>
          toast(next === "published" ? "Metric published" : "Unpublished — back to draft", { description: metric.name }),
        onError: (err) => toast.error("Couldn't update metric", { description: err.message }),
      },
    );
  }

  function toggleActive() {
    setActive.mutate(
      { id: metric.id, active: !metric.active },
      {
        onSuccess: () => toast(metric.active ? "Metric deactivated" : "Metric activated", { description: metric.name }),
        onError: (err) => toast.error("Couldn't update metric", { description: err.message }),
      },
    );
  }

  function duplicate() {
    duplicateMetric.mutate(metric.id, {
      onSuccess: (created) => toast("Metric duplicated", { description: created.name }),
      onError: (err) => toast.error("Couldn't duplicate metric", { description: err.message }),
    });
  }

  function handleDelete() {
    deleteMetric.mutate(metric.id, {
      onSuccess: () => {
        setConfirmDelete(false);
        toast("Metric deleted", { description: metric.name });
      },
      onError: (err) => {
        setConfirmDelete(false);
        if (err instanceof ApiError && err.status === 409) {
          toast.error("Can't delete — metric is in use", {
            description: "Unpublish and remove it from every surface first, or leave it archived (inactive).",
          });
          return;
        }
        toast.error("Couldn't delete metric", { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={`Actions for ${metric.name}`} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> Edit
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={togglePublish}>
            <SendIcon className="size-4" /> {metric.status === "published" ? "Unpublish" : "Publish"}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={toggleActive}>
            {metric.active ? (
              <>
                <PauseIcon className="size-4" /> Deactivate (archive)
              </>
            ) : (
              <>
                <PlayIcon className="size-4" /> Activate
              </>
            )}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={duplicate}>
            <CopyIcon className="size-4" /> Duplicate
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirmDelete(true)}>
            <Trash2Icon className="size-4" /> Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete &ldquo;{metric.name}&rdquo;?</DialogTitle>
            <DialogDescription>
              This is a hard delete. Published metrics or ones exposed on any surface can&apos;t be deleted —
              unpublish and clear its Show-in targets first, or just deactivate it instead.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDelete}>
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

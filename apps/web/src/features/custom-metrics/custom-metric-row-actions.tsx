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
import { useCustomMetricsStore } from "@/stores/custom-metrics";
import type { CustomMetric } from "@/lib/mock/custom-metrics";

export function CustomMetricRowActions({ metric, onEdit }: { metric: CustomMetric; onEdit: () => void }) {
  const updateMetric = useCustomMetricsStore((s) => s.updateMetric);
  const setActive = useCustomMetricsStore((s) => s.setActive);
  const deleteMetric = useCustomMetricsStore((s) => s.deleteMetric);
  const addMetric = useCustomMetricsStore((s) => s.addMetric);
  const [confirmDelete, setConfirmDelete] = React.useState(false);

  function togglePublish() {
    const next = metric.status === "published" ? "draft" : "published";
    updateMetric(metric.id, {
      name: metric.name,
      group: metric.group,
      formula: metric.formula,
      format: metric.format,
      targets: next === "draft" ? [] : metric.targets,
      status: next,
    });
    toast(next === "published" ? "Metric published" : "Unpublished — back to draft", { description: metric.name });
  }

  function toggleActive() {
    setActive(metric.id, !metric.active);
    toast(metric.active ? "Metric deactivated" : "Metric activated", { description: metric.name });
  }

  function duplicate() {
    addMetric(
      {
        name: `${metric.name} (Copy)`,
        group: metric.group,
        formula: metric.formula,
        format: metric.format,
        targets: [],
        status: "draft",
      },
      metric.createdByMemberId,
    );
    toast("Metric duplicated", { description: `${metric.name} (Copy)` });
  }

  function handleDelete() {
    const deleted = deleteMetric(metric.id);
    setConfirmDelete(false);
    if (deleted) {
      toast("Metric deleted", { description: metric.name });
    } else {
      toast("Can't delete — metric is in use", {
        description: "Unpublish and remove it from every surface first, or leave it archived (inactive).",
      });
    }
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

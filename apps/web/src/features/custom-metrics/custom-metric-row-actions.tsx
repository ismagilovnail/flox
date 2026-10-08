"use client";

import * as React from "react";
import { toast } from "sonner";
import { CopyIcon, MoreHorizontalIcon, PauseIcon, PencilIcon, PlayIcon, SendIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation(["customMetrics", "common"]);
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
          toast(next === "published" ? t("toasts.published") : t("toasts.unpublished"), { description: metric.name }),
        onError: (err) => toast.error(t("toasts.updateError"), { description: err.message }),
      },
    );
  }

  function toggleActive() {
    setActive.mutate(
      { id: metric.id, active: !metric.active },
      {
        onSuccess: () => toast(metric.active ? t("toasts.deactivated") : t("toasts.activated"), { description: metric.name }),
        onError: (err) => toast.error(t("toasts.updateError"), { description: err.message }),
      },
    );
  }

  function duplicate() {
    duplicateMetric.mutate(metric.id, {
      onSuccess: (created) => toast(t("toasts.duplicated"), { description: created.name }),
      onError: (err) => toast.error(t("toasts.duplicateError"), { description: err.message }),
    });
  }

  function handleDelete() {
    deleteMetric.mutate(metric.id, {
      onSuccess: () => {
        setConfirmDelete(false);
        toast(t("toasts.deleted"), { description: metric.name });
      },
      onError: (err) => {
        setConfirmDelete(false);
        if (err instanceof ApiError && err.status === 409) {
          toast.error(t("toasts.inUse"), { description: t("toasts.inUseDescription") });
          return;
        }
        toast.error(t("toasts.deleteError"), { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={t("actions.aria", { name: metric.name })} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> {t("common:actions.edit")}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={togglePublish}>
            <SendIcon className="size-4" /> {metric.status === "published" ? t("actions.unpublish") : t("actions.publish")}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={toggleActive}>
            {metric.active ? (
              <>
                <PauseIcon className="size-4" /> {t("actions.deactivate")}
              </>
            ) : (
              <>
                <PlayIcon className="size-4" /> {t("actions.activate")}
              </>
            )}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={duplicate}>
            <CopyIcon className="size-4" /> {t("common:actions.duplicate")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={() => setConfirmDelete(true)}>
            <Trash2Icon className="size-4" /> {t("common:actions.delete")}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("actions.deleteTitle", { name: metric.name })}</DialogTitle>
            <DialogDescription>{t("actions.deleteDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleDelete}>
              {t("common:actions.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

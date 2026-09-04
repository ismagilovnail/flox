"use client";

import * as React from "react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { ChevronDownIcon, CopyIcon, GripVerticalIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Switch } from "@/components/ui/switch";
import { Tag } from "@/components/ui/tag";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { hydrateRootFilter, type ApiFlow, type StreamSet } from "@/lib/api/stream-sets";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import type { Landing } from "@/lib/api/landings";
import type { Pwa } from "@/lib/api/pwa";
import type { Postlanding } from "@/lib/api/postlanding";
import type { Pixel } from "@/lib/api/pixels";
import { FilterTreeSummary } from "@/features/stream-sets/filter-tree-summary";
import { StreamSetEditor, type StreamSetFormValues } from "@/features/stream-sets/stream-set-editor";

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

export function StreamSetRow({
  streamSet,
  networks,
  offers,
  landings,
  pwas,
  postlandings,
  pixels,
  defaultExpanded = false,
  disabled = false,
  onSubmit,
  onDuplicate,
  onDelete,
  onToggleStatus,
}: {
  streamSet: StreamSet;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  pixels: Pixel[];
  defaultExpanded?: boolean;
  /** Archived campaigns don't route traffic — the row stays expandable
   * for viewing, but reordering, toggling, duplicating, deleting, and
   * editing lock instead of looking saved while they can't matter. */
  disabled?: boolean;
  onSubmit: (values: StreamSetFormValues) => void;
  onDuplicate: () => void;
  onDelete: () => void;
  onToggleStatus: () => void;
}) {
  const { t } = useTranslation(["streamSets", "common"]);
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: streamSet.id,
  });
  const [expanded, setExpanded] = React.useState(defaultExpanded);
  const [confirmDelete, setConfirmDelete] = React.useState(false);

  // Hydrated only for display (React keys + filter-chip rendering) — this
  // summary line never edits the tree, so ids are throwaway, not addressed
  // by any mutation helper the way the inline editor's own are.
  const hydratedRoot = React.useMemo(() => hydrateRootFilter(streamSet.rootFilter), [streamSet.rootFilter]);

  const weightSum = streamSet.flows.reduce((sum, f) => sum + f.weight, 0);

  function destinationLabel(flow: ApiFlow) {
    const destination = flow.destination;
    if (destination.kind === "redirect") return t("flowDestination.redirect");
    return offers.find((o) => o.id === destination.offerId)?.name ?? t("row.noOffer");
  }

  function handleSubmit(values: StreamSetFormValues) {
    onSubmit(values);
  }

  return (
    <Card
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`gap-0 overflow-hidden ring-1 ring-border ${isDragging ? "opacity-60" : ""}`}
    >
      <div className="flex items-start gap-3 px-4 py-4">
        <button
          type="button"
          aria-label={t("row.reorderAria", { name: streamSet.name })}
          disabled={disabled}
          className="mt-0.5 cursor-grab touch-none text-muted-foreground hover:text-foreground active:cursor-grabbing disabled:cursor-not-allowed disabled:opacity-40"
          {...(disabled ? {} : attributes)}
          {...(disabled ? {} : listeners)}
        >
          <GripVerticalIcon className="size-4" />
        </button>

        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <span className="font-mono text-xs text-muted-foreground">#{streamSet.priority}</span>
              <button
                type="button"
                onClick={() => setExpanded((v) => !v)}
                className="font-medium text-foreground hover:underline"
              >
                {streamSet.name}
              </button>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                size="sm"
                checked={streamSet.status === "active"}
                onCheckedChange={onToggleStatus}
                disabled={disabled}
                aria-label={streamSet.status === "active" ? t("row.disableAria") : t("row.enableAria")}
              />
              <Tooltip>
                <TooltipTrigger asChild>
                  <IconButton aria-label={t("row.duplicate")} size="icon-sm" disabled={disabled} onClick={onDuplicate}>
                    <CopyIcon className="size-3.5" />
                  </IconButton>
                </TooltipTrigger>
                <TooltipContent>{t("row.duplicate")}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <IconButton aria-label={t("row.delete")} size="icon-sm" disabled={disabled} onClick={() => setConfirmDelete(true)}>
                    <Trash2Icon className="size-3.5" />
                  </IconButton>
                </TooltipTrigger>
                <TooltipContent>{t("row.delete")}</TooltipContent>
              </Tooltip>
              <IconButton
                aria-label={expanded ? t("row.collapseAria", { name: streamSet.name }) : t("row.expandAria", { name: streamSet.name })}
                size="icon-sm"
                onClick={() => setExpanded((v) => !v)}
              >
                <ChevronDownIcon className={`size-3.5 transition-transform ${expanded ? "rotate-180" : ""}`} />
              </IconButton>
            </div>
          </div>

          <FilterTreeSummary root={hydratedRoot} />

          <div className="flex flex-wrap items-center gap-1.5">
            {streamSet.flows.map((f) => (
              <Tag key={f.id} title={destinationLabel(f)}>
                {f.name} · {weightSum > 0 ? ((f.weight / weightSum) * 100).toFixed(0) : 0}%
              </Tag>
            ))}
            <Badge variant={streamSet.status === "active" ? "success" : "secondary"}>
              {t(`status.${streamSet.status}`, { ns: "common" })}
            </Badge>
          </div>
        </div>
      </div>

      {expanded && (
        <StreamSetEditor
          key={streamSet.id}
          defaultValues={toFormValues(streamSet)}
          networks={networks}
          offers={offers}
          landings={landings}
          pwas={pwas}
          postlandings={postlandings}
          pixels={pixels}
          submitLabel={t("form.submitEdit")}
          onSubmit={handleSubmit}
          disabled={disabled}
        />
      )}

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("row.deleteConfirmTitle", { name: streamSet.name })}</DialogTitle>
            <DialogDescription>{t("row.deleteConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setConfirmDelete(false);
                onDelete();
              }}
            >
              {t("row.deleteConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

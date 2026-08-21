"use client";

import * as React from "react";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { CopyIcon, GripVerticalIcon, MoreHorizontalIcon, PencilIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { FilterChip } from "@/components/ui/filter-chip";
import { FilterGroup } from "@/components/ui/filter-group";
import { IconButton } from "@/components/ui/icon-button";
import { Switch } from "@/components/ui/switch";
import { Tag } from "@/components/ui/tag";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { countConditions, describeFilterTree } from "@/lib/filters";
import { hydrateRootFilter, type ApiFlow, type StreamSet } from "@/lib/api/stream-sets";
import type { Offer } from "@/lib/api/offers";

export function StreamSetRow({
  streamSet,
  offers,
  onEdit,
  onDuplicate,
  onToggleStatus,
}: {
  streamSet: StreamSet;
  offers: Offer[];
  onEdit: () => void;
  onDuplicate: () => void;
  onToggleStatus: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: streamSet.id,
  });

  // Hydrated only for display (React keys + filter-chip rendering) — this
  // row never edits the tree, so ids are throwaway, not addressed by any
  // mutation helper the way filter-group-builder.tsx's are.
  const hydratedRoot = React.useMemo(() => hydrateRootFilter(streamSet.rootFilter), [streamSet.rootFilter]);

  const weightSum = streamSet.flows.reduce((sum, f) => sum + f.weight, 0);

  function destinationLabel(flow: ApiFlow) {
    const destination = flow.destination;
    if (destination.kind === "redirect") return "Redirect";
    return offers.find((o) => o.id === destination.offerId)?.name ?? "No offer";
  }

  return (
    <Card
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`ring-1 ring-border ${isDragging ? "opacity-60" : ""}`}
    >
      <div className="flex items-start gap-3 px-4">
        <button
          type="button"
          aria-label={`Reorder ${streamSet.name}`}
          className="mt-0.5 cursor-grab touch-none text-muted-foreground hover:text-foreground active:cursor-grabbing"
          {...attributes}
          {...listeners}
        >
          <GripVerticalIcon className="size-4" />
        </button>

        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <span className="font-mono text-xs text-muted-foreground">#{streamSet.priority}</span>
              <button type="button" onClick={onEdit} className="font-medium text-foreground hover:underline">
                {streamSet.name}
              </button>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                size="sm"
                checked={streamSet.status === "active"}
                onCheckedChange={onToggleStatus}
                aria-label={streamSet.status === "active" ? "Disable stream set" : "Enable stream set"}
              />
              <IconButton aria-label={`Edit ${streamSet.name}`} size="icon-sm" onClick={onEdit}>
                <PencilIcon className="size-3.5" />
              </IconButton>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <IconButton aria-label={`More actions for ${streamSet.name}`} size="icon-sm">
                    <MoreHorizontalIcon className="size-3.5" />
                  </IconButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={onDuplicate}>
                    <CopyIcon className="size-4" /> Duplicate
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          {hydratedRoot.children.length > 0 ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="w-fit">
                  <FilterGroup joiner={hydratedRoot.joiner}>
                    {hydratedRoot.children.map((child) =>
                      child.type === "condition" ? (
                        <FilterChip
                          key={child.id}
                          field={child.field}
                          operator={child.operator.replace(/_/g, " ").toLowerCase()}
                          value={child.operator === "BETWEEN" ? `${child.value}–${child.valueTo}` : child.value || "—"}
                        />
                      ) : (
                        <Badge key={child.id} variant="outline">
                          {child.joiner === "AND" ? "match all" : "match any"} ({countConditions(child)})
                        </Badge>
                      ),
                    )}
                  </FilterGroup>
                </div>
              </TooltipTrigger>
              <TooltipContent>{describeFilterTree(hydratedRoot)}</TooltipContent>
            </Tooltip>
          ) : (
            <p className="text-xs text-muted-foreground">No filters — matches all traffic</p>
          )}

          <div className="flex flex-wrap items-center gap-1.5">
            {streamSet.flows.map((f) => (
              <Tag key={f.id} title={destinationLabel(f)}>
                {f.name} · {weightSum > 0 ? ((f.weight / weightSum) * 100).toFixed(0) : 0}%
              </Tag>
            ))}
            <Badge variant={streamSet.status === "active" ? "success" : "secondary"}>{streamSet.status}</Badge>
          </div>
        </div>
      </div>
    </Card>
  );
}

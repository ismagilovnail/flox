"use client";

import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { CopyIcon, GripVerticalIcon, MoreHorizontalIcon, PencilIcon, RadioTowerIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { FilterChip } from "@/components/ui/filter-chip";
import { FilterGroup } from "@/components/ui/filter-group";
import { IconButton } from "@/components/ui/icon-button";
import { Switch } from "@/components/ui/switch";
import { Tag } from "@/components/ui/tag";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { StreamSet } from "@/lib/mock/stream-sets";

export function StreamSetRow({
  streamSet,
  onEdit,
  onDuplicate,
  onToggleStatus,
}: {
  streamSet: StreamSet;
  onEdit: () => void;
  onDuplicate: () => void;
  onToggleStatus: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: streamSet.id,
  });

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

          {streamSet.filters.length > 0 ? (
            <FilterGroup joiner={streamSet.joiner}>
              {streamSet.filters.map((f) => (
                <FilterChip key={f.id} field={f.field} operator={f.operator.replace(/_/g, " ").toLowerCase()} value={f.value || "—"} />
              ))}
            </FilterGroup>
          ) : (
            <p className="text-xs text-muted-foreground">No filters — matches all traffic</p>
          )}

          <div className="flex flex-wrap items-center gap-1.5">
            {streamSet.flows.map((f) => (
              <Tag key={f.id}>
                {f.name} · {f.weight}%
              </Tag>
            ))}
            {streamSet.pixels.length > 0 && (
              <Badge variant="outline">
                <RadioTowerIcon className="size-3" /> {streamSet.pixels.length} pixel
                {streamSet.pixels.length > 1 ? "s" : ""}
              </Badge>
            )}
            <Badge variant={streamSet.status === "active" ? "success" : "secondary"}>{streamSet.status}</Badge>
          </div>
        </div>
      </div>
    </Card>
  );
}

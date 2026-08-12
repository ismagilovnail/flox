"use client";

import * as React from "react";
import { CheckIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { TAG_COLORS, randomTagColor, tagColorHex, type Tag, type TagColorId, type TaggableEntityType } from "@/lib/mock/tags";
import { useTagsStore } from "@/stores/tags";
import { useEntityTags } from "@/features/tags/use-entity-tags";

function TagEditRow({
  tag,
  onSave,
  onDelete,
  onCancel,
}: {
  tag: Tag;
  onSave: (input: { name: string; color: TagColorId }) => void;
  onDelete: () => void;
  onCancel: () => void;
}) {
  const [name, setName] = React.useState(tag.name);
  const [color, setColor] = React.useState<TagColorId>(tag.color);

  return (
    <div className="flex flex-col gap-2 rounded-md border border-border p-2">
      <Input value={name} onChange={(e) => setName(e.target.value)} className="h-7 text-xs" autoFocus />
      <div className="flex flex-wrap gap-1.5">
        {TAG_COLORS.map((c) => (
          <button
            key={c.id}
            type="button"
            aria-label={c.id}
            onClick={() => setColor(c.id)}
            className={cn(
              "size-4 rounded-full ring-offset-2 ring-offset-popover transition-shadow",
              color === c.id && "ring-2 ring-foreground",
            )}
            style={{ backgroundColor: c.hex }}
          />
        ))}
      </div>
      <div className="flex items-center justify-between gap-1">
        <IconButton aria-label="Delete tag" size="icon-sm" variant="ghost" className="text-danger" onClick={onDelete}>
          <Trash2Icon className="size-3.5" />
        </IconButton>
        <div className="flex gap-1.5">
          <Button type="button" size="sm" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="button" size="sm" disabled={!name.trim()} onClick={() => onSave({ name: name.trim(), color })}>
            Save
          </Button>
        </div>
      </div>
    </div>
  );
}

export function TagPickerPopover({
  entityType,
  entityId,
  trigger,
  open: openProp,
  onOpenChange,
}: {
  entityType: TaggableEntityType;
  entityId: string;
  trigger: React.ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const [openState, setOpenState] = React.useState(false);
  const open = openProp ?? openState;
  const setOpen = onOpenChange ?? setOpenState;

  const [query, setQuery] = React.useState("");
  const [editingId, setEditingId] = React.useState<string | null>(null);

  const allTags = useTagsStore((s) => s.tags);
  const setEntityTags = useTagsStore((s) => s.setEntityTags);
  const createTag = useTagsStore((s) => s.createTag);
  const updateTag = useTagsStore((s) => s.updateTag);
  const deleteTag = useTagsStore((s) => s.deleteTag);

  const assignedTags = useEntityTags(entityType, entityId);
  const assignedIds = React.useMemo(() => new Set(assignedTags.map((t) => t.id)), [assignedTags]);

  const trimmed = query.trim();
  const filtered = allTags.filter((t) => t.name.toLowerCase().includes(trimmed.toLowerCase()));
  const exactMatch = allTags.some((t) => t.name.toLowerCase() === trimmed.toLowerCase());

  function toggle(tagId: string) {
    const next = assignedIds.has(tagId)
      ? assignedTags.filter((t) => t.id !== tagId).map((t) => t.id)
      : [...assignedTags.map((t) => t.id), tagId];
    setEntityTags(entityType, entityId, next);
  }

  function quickCreate() {
    if (!trimmed) return;
    const id = createTag(trimmed, randomTagColor());
    setEntityTags(entityType, entityId, [...assignedTags.map((t) => t.id), id]);
    setQuery("");
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setQuery("");
          setEditingId(null);
        }
      }}
    >
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent align="start" className="w-64 p-2">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search or create tag..."
          className="mb-2 h-8"
        />
        <div className="flex max-h-56 flex-col gap-0.5 overflow-y-auto">
          {filtered.map((tag) =>
            editingId === tag.id ? (
              <TagEditRow
                key={tag.id}
                tag={tag}
                onSave={(input) => {
                  updateTag(tag.id, input);
                  setEditingId(null);
                }}
                onDelete={() => {
                  deleteTag(tag.id);
                  setEditingId(null);
                }}
                onCancel={() => setEditingId(null)}
              />
            ) : (
              <div key={tag.id} className="group flex items-center gap-2 rounded-md px-1.5 py-1 hover:bg-muted">
                <button
                  type="button"
                  onClick={() => toggle(tag.id)}
                  className="flex flex-1 items-center gap-2 text-left"
                >
                  <span
                    className={cn(
                      "flex size-4 shrink-0 items-center justify-center rounded-sm border border-input",
                      assignedIds.has(tag.id) && "border-primary bg-primary text-primary-foreground",
                    )}
                  >
                    {assignedIds.has(tag.id) && <CheckIcon className="size-3" />}
                  </span>
                  <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: tagColorHex(tag.color) }} />
                  <span className="flex-1 truncate text-sm">{tag.name}</span>
                </button>
                <IconButton
                  aria-label={`Edit ${tag.name}`}
                  size="icon-sm"
                  variant="ghost"
                  className="opacity-0 group-hover:opacity-100"
                  onClick={() => setEditingId(tag.id)}
                >
                  <PencilIcon className="size-3.5" />
                </IconButton>
              </div>
            ),
          )}

          {trimmed && !exactMatch && (
            <button
              type="button"
              onClick={quickCreate}
              className="flex items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <PlusIcon className="size-3.5" /> Create &ldquo;{trimmed}&rdquo;
            </button>
          )}

          {filtered.length === 0 && !trimmed && (
            <p className="px-1.5 py-2 text-xs text-muted-foreground">No tags yet — type a name to create one.</p>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}

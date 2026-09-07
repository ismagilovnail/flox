"use client";

import * as React from "react";
import { CheckIcon, PlusIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { randomTagColor, tagColorHex, type TaggableEntityType } from "@/lib/tags";
import { usePermission } from "@/hooks/use-auth";
import {
  EMPTY_TAG_ASSIGNMENTS,
  EMPTY_TAGS,
  useBulkEditTags,
  useCreateTag,
  useTagAssignments,
  useTags,
} from "@/hooks/use-tags";

/** §30.6 bulk semantics: pre-check only tags present on EVERY selected entity
 * (the intersection). Newly checked tags get added to every selected entity;
 * unchecking a previously-common tag removes it from every selected entity.
 * Tags that were only on some of the selection, left untouched, stay exactly
 * as they were per-entity. */
export function BulkTagDialog({
  entityType,
  entityIds,
  open,
  onOpenChange,
  onApplied,
}: {
  entityType: TaggableEntityType;
  entityIds: string[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApplied: () => void;
}) {
  // content.write — same key apps/internal/tags.Handler requires for
  // create and bulkEdit alike.
  const canWrite = usePermission("content.write");
  const tags = useTags().data?.tags ?? EMPTY_TAGS;
  const assignments = useTagAssignments(entityType).data?.assignments ?? EMPTY_TAG_ASSIGNMENTS;
  const bulkEditMutation = useBulkEditTags();
  const createTagMutation = useCreateTag();

  const [query, setQuery] = React.useState("");

  // The parent only renders this component while the dialog is open (conditional
  // `{bulkTarget && <BulkTagDialog .../>}`), so it fully unmounts on close and
  // remounts fresh on reopen — a lazy initializer is enough to seed `checked`
  // once per open, no effect/resync needed. `assignments` is already warm by
  // then: the same list page's own TagBadgeList cells render first and mount
  // the identical useTagAssignments(entityType) query.
  const commonTagIds = React.useMemo(() => {
    if (entityIds.length === 0) return new Set<string>();
    const perEntity = entityIds.map(
      (id) => new Set(assignments.filter((a) => a.entityId === id).map((a) => a.tagId)),
    );
    return perEntity.reduce((acc, set) => new Set([...acc].filter((id) => set.has(id))));
  }, [assignments, entityIds]);

  const [checked, setChecked] = React.useState<Set<string>>(() => new Set(commonTagIds));

  function toggle(tagId: string) {
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(tagId)) next.delete(tagId);
      else next.add(tagId);
      return next;
    });
  }

  const trimmed = query.trim();
  const filtered = tags.filter((t) => t.name.toLowerCase().includes(trimmed.toLowerCase()));
  const exactMatch = tags.some((t) => t.name.toLowerCase() === trimmed.toLowerCase());

  async function quickCreate() {
    if (!trimmed) return;
    const created = await createTagMutation.mutateAsync({ name: trimmed, color: randomTagColor() });
    setChecked((prev) => new Set(prev).add(created.id));
    setQuery("");
  }

  function apply() {
    const toAdd = [...checked].filter((id) => !commonTagIds.has(id));
    const toRemove = [...commonTagIds].filter((id) => !checked.has(id));
    bulkEditMutation.mutate({ entityType, entityIds, toAdd, toRemove });
    onApplied();
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit tags for {entityIds.length} items</DialogTitle>
          <DialogDescription>
            Only tags on every selected item are pre-checked. Check a tag to add it to all; uncheck a pre-checked
            tag to remove it from all. Tags only some items have are left as-is.
          </DialogDescription>
        </DialogHeader>

        <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search or create tag..." />

        <div className="flex max-h-64 flex-col gap-0.5 overflow-y-auto">
          {filtered.map((tag) => {
            const isChecked = checked.has(tag.id);
            return (
              <button
                key={tag.id}
                type="button"
                onClick={() => toggle(tag.id)}
                disabled={!canWrite}
                className="flex items-center gap-2 rounded-md px-1.5 py-1.5 text-left hover:bg-muted disabled:cursor-not-allowed disabled:opacity-70"
              >
                <span
                  className={cn(
                    "flex size-4 shrink-0 items-center justify-center rounded-sm border border-input",
                    isChecked && "border-primary bg-primary text-primary-foreground",
                  )}
                >
                  {isChecked && <CheckIcon className="size-3" />}
                </span>
                <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: tagColorHex(tag.color) }} />
                <span className="flex-1 truncate text-sm">{tag.name}</span>
              </button>
            );
          })}
          {canWrite && trimmed && !exactMatch && (
            <button
              type="button"
              onClick={quickCreate}
              className="flex items-center gap-2 rounded-md px-1.5 py-1.5 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              <PlusIcon className="size-3.5" /> Create &ldquo;{trimmed}&rdquo;
            </button>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" onClick={apply} disabled={!canWrite}>
            Apply
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

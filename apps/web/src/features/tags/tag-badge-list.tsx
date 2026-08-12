"use client";

import * as React from "react";
import { PlusIcon } from "lucide-react";

import { tagColorHex, type TaggableEntityType } from "@/lib/mock/tags";
import { useEntityTags } from "@/features/tags/use-entity-tags";
import { TagPickerPopover } from "@/features/tags/tag-picker-popover";

/** The one "Tags column" cell renderer, reused across every taggable list
 * (§30.6) — ≤3 tags shown fully, >3 shows first 3 + "+N", none shows an
 * "Add tags" affordance. Clicking anywhere opens the same picker used by
 * each row's "Manage Tags" action. */
export function TagBadgeList({ entityType, entityId }: { entityType: TaggableEntityType; entityId: string }) {
  const [open, setOpen] = React.useState(false);
  const tags = useEntityTags(entityType, entityId);
  const visible = tags.slice(0, 3);
  const overflow = tags.length - visible.length;

  return (
    <TagPickerPopover
      entityType={entityType}
      entityId={entityId}
      open={open}
      onOpenChange={setOpen}
      trigger={
        <button type="button" onClick={() => setOpen(true)} className="flex flex-wrap items-center gap-1">
          {tags.length === 0 ? (
            <span className="flex items-center gap-1 rounded-full border border-dashed border-border px-2 py-0.5 text-xs text-muted-foreground hover:border-foreground/40 hover:text-foreground">
              <PlusIcon className="size-3" /> Add tags
            </span>
          ) : (
            <>
              {visible.map((tag) => (
                <span key={tag.id} className="flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs">
                  <span className="size-1.5 shrink-0 rounded-full" style={{ backgroundColor: tagColorHex(tag.color) }} />
                  {tag.name}
                </span>
              ))}
              {overflow > 0 && <span className="text-xs text-muted-foreground">+{overflow}</span>}
            </>
          )}
        </button>
      }
    />
  );
}

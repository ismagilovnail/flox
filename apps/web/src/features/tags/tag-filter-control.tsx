"use client";

import * as React from "react";
import { CheckIcon, ChevronDownIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { tagColorHex } from "@/lib/mock/tags";
import { useTagsStore } from "@/stores/tags";

/** The one tag-filter implementation, reused across every taggable list
 * (§30.6). OR semantics — filtering itself happens in filter-by-tags.ts,
 * this component only owns the selected-id state UI. */
export function TagFilterControl({
  selected,
  onChange,
}: {
  selected: string[];
  onChange: (tagIds: string[]) => void;
}) {
  const tags = useTagsStore((s) => s.tags);
  const [open, setOpen] = React.useState(false);

  function toggle(tagId: string) {
    onChange(selected.includes(tagId) ? selected.filter((id) => id !== tagId) : [...selected, tagId]);
  }

  const summary =
    selected.length === 0
      ? "All tags"
      : selected.length <= 2
        ? selected.map((id) => tags.find((t) => t.id === id)?.name ?? id).join(", ")
        : `${selected.length} tags`;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-2 font-normal">
          {selected.length > 0 && (
            <span className="flex -space-x-1">
              {selected.slice(0, 3).map((id) => (
                <span
                  key={id}
                  className="size-2 rounded-full ring-1 ring-background"
                  style={{ backgroundColor: tagColorHex(tags.find((t) => t.id === id)?.color ?? "gray") }}
                />
              ))}
            </span>
          )}
          <span className="text-muted-foreground">Tags:</span> {summary}
          <ChevronDownIcon className="size-3.5 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-56 p-1">
        {tags.length === 0 && <p className="px-2 py-2 text-xs text-muted-foreground">No tags yet.</p>}
        {tags.map((tag) => {
          const checked = selected.includes(tag.id);
          return (
            <button
              key={tag.id}
              type="button"
              onClick={() => toggle(tag.id)}
              className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
            >
              <span
                className={cn(
                  "flex size-4 shrink-0 items-center justify-center rounded-sm border border-input",
                  checked && "border-primary bg-primary text-primary-foreground",
                )}
              >
                {checked && <CheckIcon className="size-3" />}
              </span>
              <span className="size-2 shrink-0 rounded-full" style={{ backgroundColor: tagColorHex(tag.color) }} />
              <span className="flex-1 truncate">{tag.name}</span>
            </button>
          );
        })}
      </PopoverContent>
    </Popover>
  );
}

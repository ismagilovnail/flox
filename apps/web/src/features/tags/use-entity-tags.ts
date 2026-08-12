"use client";

import * as React from "react";

import { useTagsStore } from "@/stores/tags";
import type { Tag, TaggableEntityType } from "@/lib/mock/tags";

/** Selects the raw, stable `tags`/`assignments` arrays and filters in a
 * `useMemo` — never call a store method that itself returns a freshly
 * `.filter()`'d array as the selector; that breaks useSyncExternalStore's
 * snapshot-stability check and infinite-loops the render (see Phase 13). */
export function useEntityTags(entityType: TaggableEntityType, entityId: string): Tag[] {
  const tags = useTagsStore((s) => s.tags);
  const assignments = useTagsStore((s) => s.assignments);

  return React.useMemo(() => {
    const tagIds = new Set(
      assignments.filter((a) => a.entityType === entityType && a.entityId === entityId).map((a) => a.tagId),
    );
    return tags.filter((t) => tagIds.has(t.id));
  }, [tags, assignments, entityType, entityId]);
}

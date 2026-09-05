"use client";

import * as React from "react";

import { EMPTY_TAGS, EMPTY_TAG_ASSIGNMENTS, useTagAssignments, useTags } from "@/hooks/use-tags";
import type { Tag, TaggableEntityType } from "@/lib/tags";

/** Derives the tags on one entity from the two org-wide queries
 * (`useTags`, `useTagAssignments`) via a `useMemo` — never derive from a
 * query result that itself returns a freshly `.filter()`'d array as a
 * selector; that's the same snapshot-stability trap the old zustand
 * version of this hook was written to avoid (see Phase 13). The
 * `EMPTY_TAGS`/`EMPTY_TAG_ASSIGNMENTS` fallbacks below are referentially
 * stable across renders — a plain `?? []` here would reintroduce that
 * exact trap while either query is still loading. */
export function useEntityTags(entityType: TaggableEntityType, entityId: string): Tag[] {
  const tags = useTags().data?.tags ?? EMPTY_TAGS;
  const assignments = useTagAssignments(entityType).data?.assignments ?? EMPTY_TAG_ASSIGNMENTS;

  return React.useMemo(() => {
    const tagIds = new Set(assignments.filter((a) => a.entityId === entityId).map((a) => a.tagId));
    return tags.filter((t) => tagIds.has(t.id));
  }, [tags, assignments, entityId]);
}
